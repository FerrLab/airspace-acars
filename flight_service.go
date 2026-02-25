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
	posIntervalLow    = 500 * time.Millisecond  // below 10,000 ft AGL
	posIntervalHigh   = 2 * time.Second          // at/above 10,000 ft AGL
	posIntervalStatic = 60 * time.Second          // position unchanged
	highAltThreshold  = 10_000.0
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

			// Adaptive interval: static → 60s, else altitude-based
			var newInterval time.Duration
			if !posChanged && time.Since(lastChanged) > 5*time.Second {
				newInterval = posIntervalStatic
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

func (f *FlightService) buildPositionReport(fd *FlightData) map[string]interface{} {
	f.mu.Lock()
	callsign := f.callsign
	departure := f.departure
	arrival := f.arrival
	elapsed := time.Since(f.startTime).Seconds()
	f.mu.Unlock()

	engCount := 0
	for _, e := range fd.Engines {
		if e.Running {
			engCount++
		}
	}

	gearVal := 0.0
	if fd.Controls.GearDown {
		gearVal = 1.0
	}

	return map[string]interface{}{
		"latitude":               fd.Position.Latitude,
		"longitude":              fd.Position.Longitude,
		"altitude":               fd.Position.Altitude,
		"altitudeAgl":            fd.Position.AltitudeAGL,
		"heading":                fd.Attitude.HeadingTrue,
		"pitch":                  fd.Attitude.Pitch,
		"bank":                   fd.Attitude.Roll,
		"groundSpeed":            fd.Attitude.GS,
		"indicatedAirspeed":      fd.Attitude.IAS,
		"trueAirspeed":           fd.Attitude.TAS,
		"verticalSpeed":          fd.Attitude.VS,
		"onGround":               fd.Sensors.OnGround,
		"gearControl":            gearVal,
		"flapsControl":           fd.Controls.Flaps,
		"aircraftType":           "",
		"gForce":                 0,
		"crashed":                false,
		"slewMode":               false,
		"simulationRate":         fd.Sensors.SimulationRate,
		"pauseFlag":              false,
		"enginesCount":           engCount,
		"engine1Firing":          fd.Engines[0].Running,
		"engine2Firing":          fd.Engines[1].Running,
		"engine3Firing":          fd.Engines[2].Running,
		"engine4Firing":          fd.Engines[3].Running,
		"engine1N1":              fd.Engines[0].N1,
		"engine2N1":              fd.Engines[1].N1,
		"engine3N1":              fd.Engines[2].N1,
		"engine4N1":              fd.Engines[3].N1,
		"fuelTotalQuantityWeight": 0,
		"transponderFreq":        fd.Radios.XpdrCode,
		"com1Freq":               fd.Radios.Com1,
		"com2Freq":               fd.Radios.Com2,
		"windDirection":          0,
		"windSpeed":              0,
		"pressureQNH":            fd.Altimeter * 33.8639,
		"zuluHour":               int(fd.SimTime.ZuluTime) / 3600,
		"zuluMin":                (int(fd.SimTime.ZuluTime) % 3600) / 60,
		"zuluSec":                int(fd.SimTime.ZuluTime) % 60,
		"elapsedTime":            elapsed,
		"callsign":               callsign,
		"departure":              departure,
		"arrival":                arrival,
		"timestamp":              time.Now().UTC().Format(time.RFC3339),
	}
}
