package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type FlightService struct {
	auth       *AuthService
	flightData *FlightDataService
	app        *application.App

	mu        sync.Mutex
	state     string // "idle" or "active"
	callsign  string
	departure string
	arrival   string
	startTime time.Time
	stopCh    chan struct{}
}

func NewFlightService(auth *AuthService, fd *FlightDataService) *FlightService {
	return &FlightService{
		auth:       auth,
		flightData: fd,
		state:      "idle",
	}
}

func (f *FlightService) setApp(app *application.App) {
	f.app = app
}

func (f *FlightService) GetFlightState() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *FlightService) GetBooking() (map[string]interface{}, error) {
	body, _, err := f.auth.doRequest("GET", "/api/acars/booking", nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse booking: %w", err)
	}
	return result, nil
}

func (f *FlightService) StartFlight(callsign, departure, arrival string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.state == "active" {
		return fmt.Errorf("flight already active")
	}

	payload := map[string]string{
		"callsign":  callsign,
		"departure": departure,
		"arrival":   arrival,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	_, status, err := f.auth.doRequest("POST", "/api/acars/start", payload)
	if err != nil {
		return fmt.Errorf("start flight: %w", err)
	}
	if status >= 400 {
		return fmt.Errorf("start flight: server returned %d", status)
	}

	f.state = "active"
	f.callsign = callsign
	f.departure = departure
	f.arrival = arrival
	f.startTime = time.Now()
	f.stopCh = make(chan struct{})

	go f.positionLoop(f.stopCh)

	slog.Info("flight started", "callsign", callsign, "dep", departure, "arr", arrival)

	if f.app != nil {
		f.app.Event.Emit("flight-state", "active")
	}
	return nil
}

func (f *FlightService) StopFlight() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.state != "active" {
		return fmt.Errorf("no active flight")
	}

	payload := map[string]string{
		"callsign":  f.callsign,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	_, _, err := f.auth.doRequest("POST", "/api/acars/stop", payload)
	if err != nil {
		slog.Warn("stop flight request failed", "error", err)
	}

	f.endFlight()
	slog.Info("flight stopped/cancelled")
	return nil
}

func (f *FlightService) FinishFlight() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.state != "active" {
		return fmt.Errorf("no active flight")
	}

	payload := map[string]string{
		"callsign":  f.callsign,
		"departure": f.departure,
		"arrival":   f.arrival,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	body, status, err := f.auth.doRequest("POST", "/api/acars/finish", payload)
	if err != nil {
		return fmt.Errorf("finish flight: %w", err)
	}
	if status >= 400 {
		var errResp map[string]interface{}
		json.Unmarshal(body, &errResp)
		if msg, ok := errResp["error"].(string); ok {
			return fmt.Errorf("finish flight: %s", msg)
		}
		return fmt.Errorf("finish flight: server returned %d", status)
	}

	f.endFlight()
	slog.Info("flight finished")
	return nil
}

// endFlight stops the position loop and resets state. Must be called with mu held.
func (f *FlightService) endFlight() {
	if f.stopCh != nil {
		close(f.stopCh)
		f.stopCh = nil
	}
	f.state = "idle"
	f.callsign = ""
	f.departure = ""
	f.arrival = ""

	if f.app != nil {
		f.app.Event.Emit("flight-state", "idle")
	}
}

const (
	posIntervalCritical  = 500 * time.Millisecond // airborne below 50 ft AGL (2hz)
	posIntervalLow       = 1 * time.Second        // below 10,000 ft AGL
	posIntervalHigh      = 2 * time.Second        // at/above 10,000 ft AGL
	posIntervalStatic    = 60 * time.Second       // position unchanged
	criticalAltThreshold = 50.0
	highAltThreshold     = 10_000.0
)

func (f *FlightService) positionLoop(stopCh chan struct{}) {
	ticker := time.NewTicker(posIntervalLow)
	defer ticker.Stop()

	currentInterval := posIntervalLow
	var lastLat, lastLng float64
	lastChanged := time.Now()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			fd, err := f.flightData.GetFlightDataNow()
			if err != nil {
				continue
			}

			// Detect position change
			posChanged := fd.Position.Latitude != lastLat || fd.Position.Longitude != lastLng
			if posChanged {
				lastLat = fd.Position.Latitude
				lastLng = fd.Position.Longitude
				lastChanged = time.Now()
			}

			// Adaptive interval: static → 60s, critical → 500ms, altitude-based otherwise
			var newInterval time.Duration
			if !posChanged && time.Since(lastChanged) > 5*time.Second {
				newInterval = posIntervalStatic
			} else if !fd.Sensors.OnGround && fd.Position.AltitudeAGL < criticalAltThreshold {
				newInterval = posIntervalCritical
			} else if fd.Position.AltitudeAGL >= highAltThreshold {
				newInterval = posIntervalHigh
			} else {
				newInterval = posIntervalLow
			}
			if newInterval != currentInterval {
				currentInterval = newInterval
				ticker.Reset(currentInterval)
			}

			report := f.buildPositionReport(fd)
			_, _, err = f.auth.doRequest("POST", "/api/acars/position", report)
			if err != nil {
				slog.Debug("position report failed", "error", err)
			}
		}
	}
}

// measurement wraps a numeric value with its unit of measurement.
type measurement struct {
	Value interface{} `json:"value"`
	Unit  string      `json:"unit"`
}

func m(value interface{}, unit string) measurement {
	return measurement{Value: value, Unit: unit}
}

func (f *FlightService) buildPositionReport(fd *FlightData) map[string]interface{} {
	f.mu.Lock()
	callsign := f.callsign
	departure := f.departure
	arrival := f.arrival
	elapsed := time.Since(f.startTime).Seconds()
	f.mu.Unlock()

	zuluSec := int(fd.SimTime.ZuluTime)

	engines := make([]map[string]interface{}, len(fd.Engines))
	for i, e := range fd.Engines {
		engines[i] = map[string]interface{}{
			"running":   e.Running,
			"n1":        m(e.N1, "%"),
			"n2":        m(e.N2, "%"),
			"throttle":  m(e.ThrottlePos, "%"),
			"mixture":   m(e.MixturePos, "%"),
			"propeller": m(e.PropPos, "%"),
		}
	}

	doors := make([]map[string]interface{}, len(fd.Doors))
	for i, d := range fd.Doors {
		doors[i] = map[string]interface{}{
			"open": m(d.OpenRatio, "ratio"),
		}
	}

	return map[string]interface{}{
		"callsign":    callsign,
		"departure":   departure,
		"arrival":     arrival,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"elapsedTime": m(elapsed, "s"),
		"position": map[string]interface{}{
			"latitude":    m(fd.Position.Latitude, "deg"),
			"longitude":   m(fd.Position.Longitude, "deg"),
			"altitude":    m(fd.Position.Altitude, "ft"),
			"altitudeAgl": m(fd.Position.AltitudeAGL, "ft"),
		},
		"attitude": map[string]interface{}{
			"pitch":       m(fd.Attitude.Pitch, "deg"),
			"roll":        m(fd.Attitude.Roll, "deg"),
			"headingTrue": m(fd.Attitude.HeadingTrue, "deg"),
			"headingMag":  m(fd.Attitude.HeadingMag, "deg"),
			"vs":          m(fd.Attitude.VS, "fpm"),
			"ias":         m(fd.Attitude.IAS, "kts"),
			"tas":         m(fd.Attitude.TAS, "kts"),
			"gs":          m(fd.Attitude.GS, "kts"),
			"gForce":      m(fd.Attitude.GForce, "G"),
		},
		"engines": engines,
		"sensors": map[string]interface{}{
			"onGround":         fd.Sensors.OnGround,
			"stallWarning":     fd.Sensors.StallWarning,
			"overspeedWarning": fd.Sensors.OverspeedWarning,
			"simulationRate":   m(fd.Sensors.SimulationRate, "x"),
		},
		"radios": map[string]interface{}{
			"com1":             m(fd.Radios.Com1, "MHz"),
			"com2":             m(fd.Radios.Com2, "MHz"),
			"nav1":             m(fd.Radios.Nav1, "MHz"),
			"nav2":             m(fd.Radios.Nav2, "MHz"),
			"nav1Obs":          m(fd.Radios.Nav1OBS, "deg"),
			"nav2Obs":          m(fd.Radios.Nav2OBS, "deg"),
			"transponderCode":  m(fd.Radios.XpdrCode, ""),
			"transponderState": fd.Radios.XpdrState,
		},
		"autopilot": map[string]interface{}{
			"master":       fd.Autopilot.Master,
			"heading":      m(fd.Autopilot.Heading, "deg"),
			"altitude":     m(fd.Autopilot.Altitude, "ft"),
			"vs":           m(fd.Autopilot.VS, "fpm"),
			"speed":        m(fd.Autopilot.Speed, "kts"),
			"approachHold": fd.Autopilot.ApproachHold,
			"navLock":      fd.Autopilot.NavLock,
		},
		"altimeter": m(fd.Altimeter, "inHg"),
		"lights": map[string]interface{}{
			"beacon":  fd.Lights.Beacon,
			"strobe":  fd.Lights.Strobe,
			"landing": fd.Lights.Landing,
		},
		"controls": map[string]interface{}{
			"elevator": m(fd.Controls.Elevator, "position"),
			"aileron":  m(fd.Controls.Aileron, "position"),
			"rudder":   m(fd.Controls.Rudder, "position"),
			"flaps":    m(fd.Controls.Flaps, "%"),
			"spoilers": m(fd.Controls.Spoilers, "%"),
			"gearDown": fd.Controls.GearDown,
		},
		"apu": map[string]interface{}{
			"switchOn":  fd.APU.SwitchOn,
			"rpm":       m(fd.APU.RPMPercent, "%"),
			"genSwitch": fd.APU.GenSwitch,
			"genActive": fd.APU.GenActive,
		},
		"doors": doors,
		"simTime": map[string]interface{}{
			"zuluHour":  m(zuluSec/3600, "h"),
			"zuluMin":   m((zuluSec%3600)/60, "min"),
			"zuluSec":   m(zuluSec%60, "s"),
			"zuluDay":   m(fd.SimTime.ZuluDay, ""),
			"zuluMonth": m(fd.SimTime.ZuluMonth, ""),
			"zuluYear":  m(fd.SimTime.ZuluYear, ""),
			"localTime": m(fd.SimTime.LocalTime, "s"),
		},
		"aircraftName": fd.AircraftName,
		"weight": map[string]interface{}{
			"total": m(fd.Weight.TotalWeight, "lbs"),
			"fuel":  m(fd.Weight.FuelWeight, "lbs"),
		},
	}
}
