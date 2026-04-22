package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var (
	flightTracer = observability.Tracer("flight")
	flightMeter  = observability.Meter("flight")
)

var (
	posReportsSent, _   = flightMeter.Int64Counter("position.reports_sent",
		metric.WithDescription("Successfully sent position reports"))
	posReportsQueued, _ = flightMeter.Int64Counter("position.reports_queued",
		metric.WithDescription("Position reports queued due to failure"))
	posReportsFailed, _ = flightMeter.Int64Counter("position.reports_failed",
		metric.WithDescription("Position reports that failed to send"))
	posQueueDepth, _    = flightMeter.Int64Histogram("position.queue_depth",
		metric.WithDescription("Position report queue depth at each tick"))
	posFlushTotal, _    = flightMeter.Int64Counter("position.flush_total",
		metric.WithDescription("Flush attempts on flight end"))
	posHighResQueued, _ = flightMeter.Int64Counter("position.highres_queued",
		metric.WithDescription("High-resolution reports queued during flare"))
	posHighResDepth, _  = flightMeter.Int64Histogram("position.highres_depth",
		metric.WithDescription("High-resolution queue depth at each drain"))
)

const (
	highResInterval      = 33 * time.Millisecond
	posIntervalCritical  = 500 * time.Millisecond
	posIntervalLow       = 1 * time.Second
	posIntervalHigh      = 2 * time.Second
	posIntervalStatic    = 60 * time.Second
	flareAltThreshold    = 50.0  // enter high-res below 50ft AGL (was 10ft — too narrow)
	criticalAltThreshold = 200.0 // switch to 500ms reporting below 200ft AGL
	highAltThreshold     = 10_000.0
	maxPendingReports    = 500
	maxHighResReports    = 1500
	retryAttempts        = 4

	// minFlightDuration guards against fat-fingered finishes immediately after
	// start. Cancel/stop is unaffected — only "finish" claims the flight is complete.
	minFlightDuration = 1 * time.Minute
)

// GetFlightState returns "idle" or "active".
func (a *App) GetFlightState() string {
	a.flightMu.Lock()
	defer a.flightMu.Unlock()
	return a.state
}

// GetActiveFlightInfo returns callsign, departure and arrival for the current flight.
func (a *App) GetActiveFlightInfo() map[string]string {
	a.flightMu.Lock()
	defer a.flightMu.Unlock()
	if a.state != "active" {
		return map[string]string{}
	}
	return map[string]string{
		"callsign":  a.callsign,
		"departure": a.departure,
		"arrival":   a.arrival,
	}
}

// GetBooking fetches the current booking from the API.
func (a *App) GetBooking() (map[string]interface{}, error) {
	body, _, err := a.Airspace.DoRequest("GET", "/api/acars/booking", nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse booking: %w", err)
	}
	return result, nil
}

// GetPilot fetches the pilot profile from the API.
func (a *App) GetPilot() (map[string]interface{}, error) {
	body, _, err := a.Airspace.DoRequest("GET", "/api/v2/acars/pilot", nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse pilot: %w", err)
	}
	return result, nil
}

// StartFlight begins a new flight tracking session.
func (a *App) StartFlight(callsign, departure, arrival string) error {
	_, span := flightTracer.Start(context.Background(), "flight.start",
		trace.WithAttributes(
			attribute.String("flight.callsign", callsign),
			attribute.String("flight.departure", departure),
			attribute.String("flight.arrival", arrival),
		))
	defer span.End()

	fd, err := a.GetFlightDataNow()
	if err != nil {
		err = fmt.Errorf("simulator not connected")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if !fd.Sensors.OnGround {
		err = fmt.Errorf("aircraft must be on the ground to start a flight")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if fd.Attitude.GS >= 1.0 {
		err = fmt.Errorf("aircraft must be stationary to start a flight")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	a.flightMu.Lock()
	defer a.flightMu.Unlock()

	if a.state == "active" {
		err = fmt.Errorf("flight already active")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	payload := map[string]interface{}{
		"callsign":  callsign,
		"departure": departure,
		"arrival":   arrival,
		"timestamp": time.Now().UnixMilli(),
	}

	_, status, err := a.Airspace.DoRequest("POST", "/api/acars/start", payload)
	if err != nil {
		err = fmt.Errorf("start flight: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if status >= 400 {
		err = fmt.Errorf("start flight: server returned %d", status)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	a.state = "active"
	a.callsign = callsign
	a.departure = departure
	a.arrival = arrival
	a.startTime = time.Now()
	a.stopCh = make(chan struct{})

	go a.positionLoop(a.stopCh)

	slog.Info("flight started", "callsign", callsign, "dep", departure, "arr", arrival)
	a.UI.EmitEvent("flight-state", "active")
	return nil
}

// StopFlight cancels the active flight.
func (a *App) StopFlight() error {
	_, span := flightTracer.Start(context.Background(), "flight.stop")
	defer span.End()

	a.flightMu.Lock()
	defer a.flightMu.Unlock()

	span.SetAttributes(attribute.String("flight.callsign", a.callsign))

	if a.state != "active" {
		return fmt.Errorf("no active flight")
	}

	payload := map[string]interface{}{
		"callsign":  a.callsign,
		"timestamp": time.Now().UnixMilli(),
	}

	_, _, err := a.doRequestWithRetry("POST", "/api/acars/stop", payload)
	if err != nil {
		slog.Warn("stop flight request failed after retries", "error", err)
	}

	a.endFlight()
	slog.Info("flight stopped/cancelled")
	return nil
}

// FinishFlight completes the active flight.
func (a *App) FinishFlight() error {
	_, span := flightTracer.Start(context.Background(), "flight.finish")
	defer span.End()

	a.flightMu.Lock()
	defer a.flightMu.Unlock()

	span.SetAttributes(
		attribute.String("flight.callsign", a.callsign),
		attribute.Float64("flight.duration_sec", time.Since(a.startTime).Seconds()),
	)

	if a.state != "active" {
		err := fmt.Errorf("no active flight")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if elapsed := time.Since(a.startTime); elapsed < minFlightDuration {
		remaining := (minFlightDuration - elapsed).Round(time.Second)
		err := fmt.Errorf("flight too short to finish, please wait %s", remaining)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	payload := map[string]interface{}{
		"callsign":  a.callsign,
		"departure": a.departure,
		"arrival":   a.arrival,
		"timestamp": time.Now().UnixMilli(),
	}

	body, status, err := a.doRequestWithRetry("POST", "/api/acars/finish", payload)
	if err != nil {
		err = fmt.Errorf("finish flight: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if status >= 400 {
		var errResp map[string]interface{}
		json.Unmarshal(body, &errResp)
		if msg, ok := errResp["error"].(string); ok {
			err = fmt.Errorf("finish flight: %s", msg)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		err = fmt.Errorf("finish flight: server returned %d", status)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	a.endFlight()
	slog.Info("flight finished")
	return nil
}

func (a *App) endFlight() {
	if a.stopCh != nil {
		close(a.stopCh)
		a.stopCh = nil
	}
	a.state = "idle"
	a.callsign = ""
	a.departure = ""
	a.arrival = ""
	a.UI.EmitEvent("flight-state", "idle")
}

func (a *App) doRequestWithRetry(method, path string, body interface{}) ([]byte, int, error) {
	_, span := flightTracer.Start(context.Background(), "flight.request_with_retry",
		trace.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.path", path),
		))
	defer span.End()

	var lastErr error
	backoff := 2 * time.Second
	for attempt := range retryAttempts {
		respBody, status, err := a.Airspace.DoRequest(method, path, body)
		if err == nil {
			span.SetAttributes(
				attribute.Int("retry.attempt", attempt+1),
				attribute.String("retry.final_status", "success"),
			)
			return respBody, status, nil
		}
		lastErr = err
		if attempt < retryAttempts-1 {
			slog.Warn("request failed, retrying", "method", method, "path", path, "attempt", attempt+1, "error", err, "backoff", backoff)
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	span.SetAttributes(
		attribute.Int("retry.attempt", retryAttempts),
		attribute.String("retry.final_status", "failed"),
	)
	span.RecordError(lastErr)
	span.SetStatus(codes.Error, lastErr.Error())
	return nil, 0, fmt.Errorf("all %d attempts failed: %w", retryAttempts, lastErr)
}

// measurement wraps a numeric value with its unit of measurement.
type measurement struct {
	Value interface{} `json:"value"`
	Unit  string      `json:"unit"`
}

func m(value interface{}, unit string) measurement {
	return measurement{Value: value, Unit: unit}
}

func (a *App) buildPositionReport(fd *domain.FlightData) map[string]interface{} {
	a.flightMu.Lock()
	callsign := a.callsign
	departure := a.departure
	arrival := a.arrival
	elapsed := time.Since(a.startTime).Seconds()
	a.flightMu.Unlock()

	zuluSec := int(fd.SimTime.ZuluTime)

	engines := make([]map[string]interface{}, len(fd.Engines))
	for i, e := range fd.Engines {
		engines[i] = map[string]interface{}{
			"exists":    e.Exists,
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

	simulator := a.ConnectedAdapter()

	return map[string]interface{}{
		"acarsVersion": domain.Version,
		"simulator":    simulator,
		"callsign":     callsign,
		"departure":    departure,
		"arrival":      arrival,
		"timestamp":    time.Now().UnixMilli(),
		"elapsedTime":  m(elapsed, "s"),
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
			"paused":           fd.Sensors.Paused,
			"slew":             fd.Sensors.Slew,
			"crashed":          fd.Sensors.Crashed,
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
		"aircraftType": fd.AircraftType,
		"wind": map[string]interface{}{
			"direction": m(fd.WindDirection, "deg"),
			"speed":     m(fd.WindSpeed, "kts"),
		},
		"qnh": m(fd.QNH, "hPa"),
		"weight": map[string]interface{}{
			"total": m(fd.Weight.TotalWeight, "lbs"),
			"fuel":  m(fd.Weight.FuelWeight, "lbs"),
		},
	}
}

func intervalName(d time.Duration) string {
	switch d {
	case posIntervalCritical:
		return "critical(500ms)"
	case posIntervalLow:
		return "low(1s)"
	case posIntervalHigh:
		return "high(2s)"
	case posIntervalStatic:
		return "static(60s)"
	default:
		return d.String()
	}
}

func (a *App) positionLoop(stopCh chan struct{}) {
	ticker := time.NewTicker(posIntervalLow)
	defer ticker.Stop()

	collectTicker := time.NewTicker(time.Hour)
	collectTicker.Stop()
	defer collectTicker.Stop()

	currentInterval := posIntervalLow
	var lastLat, lastLng float64
	lastChanged := time.Now()

	var pendingReports []map[string]interface{}
	var highResQueue []map[string]interface{}
	var consecutiveFailures int
	collecting := false
	// Keep collecting for a short period after touchdown to capture rollout data
	var touchdownGrace time.Time

	slog.Info("position loop started", "interval", intervalName(currentInterval))

	for {
		select {
		case <-stopCh:
			slog.Info("position loop stopping", "collecting", collecting, "highResQueued", len(highResQueue), "pendingQueued", len(pendingReports))
			if len(highResQueue) > 0 {
				if _, _, err := a.Airspace.DoRequest("POST", "/api/v2/acars/position", highResQueue); err != nil {
					slog.Warn("failed to flush high-res reports on flight end", "count", len(highResQueue), "error", err)
				} else {
					posReportsSent.Add(context.Background(), int64(len(highResQueue)))
					slog.Info("flushed high-res reports", "count", len(highResQueue))
				}
			}
			a.flushPendingReports(pendingReports)
			return

		case <-collectTicker.C:
			if !collecting {
				continue
			}
			fd, err := a.GetFlightDataNow()
			if err != nil {
				slog.Warn("high-res collect tick: no data", "error", err)
				continue
			}
			if len(highResQueue) < maxHighResReports {
				highResQueue = append(highResQueue, a.buildPositionReport(fd))
				posHighResQueued.Add(context.Background(), 1)
			}

		case <-ticker.C:
			fd, err := a.GetFlightDataNow()
			if err != nil {
				continue
			}

			posQueueDepth.Record(context.Background(), int64(len(pendingReports)))

			posChanged := fd.Position.Latitude != lastLat || fd.Position.Longitude != lastLng
			if posChanged {
				lastLat = fd.Position.Latitude
				lastLng = fd.Position.Longitude
				lastChanged = time.Now()
			}

			// High-res flare zone detection.
			// Enter: airborne AND below flare threshold (50ft AGL).
			// Stay: keep collecting through touchdown for 5 seconds of rollout data.
			// Exit: only after on ground AND grace period expired AND GS < 40kts.
			shouldCollect := false
			if !fd.Sensors.OnGround && fd.Position.AltitudeAGL < flareAltThreshold {
				// Airborne in the flare zone
				shouldCollect = true
				touchdownGrace = time.Time{} // reset grace — haven't touched down yet
			} else if collecting && fd.Sensors.OnGround {
				// Just touched down or rolling out — start/continue grace period
				if touchdownGrace.IsZero() {
					touchdownGrace = time.Now()
					slog.Info("touchdown detected, continuing high-res collection",
						"agl", fd.Position.AltitudeAGL, "gs", fd.Attitude.GS)
				}
				// Keep collecting for 5s after touchdown or until slowed below 40kts
				if time.Since(touchdownGrace) < 5*time.Second && fd.Attitude.GS >= 40 {
					shouldCollect = true
				}
			}

			if shouldCollect && !collecting {
				collecting = true
				collectTicker.Reset(highResInterval)
				slog.Info("HIGH-RES ON: entering flare zone",
					"agl", fmt.Sprintf("%.1f", fd.Position.AltitudeAGL),
					"gs", fmt.Sprintf("%.1f", fd.Attitude.GS),
					"onGround", fd.Sensors.OnGround,
					"interval", "33ms")
			} else if !shouldCollect && collecting {
				collecting = false
				collectTicker.Stop()
				slog.Info("HIGH-RES OFF: leaving flare zone",
					"agl", fmt.Sprintf("%.1f", fd.Position.AltitudeAGL),
					"gs", fmt.Sprintf("%.1f", fd.Attitude.GS),
					"onGround", fd.Sensors.OnGround,
					"queued", len(highResQueue),
					"graceSec", fmt.Sprintf("%.1f", time.Since(touchdownGrace).Seconds()))
				touchdownGrace = time.Time{}
			}

			// Determine reporting interval
			var newInterval time.Duration
			var reason string
			if len(highResQueue) > 0 {
				newInterval = posIntervalCritical
				reason = "draining high-res queue"
			} else if !posChanged && time.Since(lastChanged) > 5*time.Second {
				newInterval = posIntervalStatic
				reason = "position static"
			} else if fd.Position.AltitudeAGL < criticalAltThreshold && (!fd.Sensors.OnGround || collecting) {
				newInterval = posIntervalCritical
				reason = fmt.Sprintf("low altitude (%.0fft AGL)", fd.Position.AltitudeAGL)
			} else if fd.Position.AltitudeAGL >= highAltThreshold {
				newInterval = posIntervalHigh
				reason = fmt.Sprintf("high altitude (%.0fft AGL)", fd.Position.AltitudeAGL)
			} else {
				newInterval = posIntervalLow
				reason = "normal"
			}
			if newInterval != currentInterval {
				slog.Info("position interval changed",
					"from", intervalName(currentInterval),
					"to", intervalName(newInterval),
					"reason", reason,
					"agl", fmt.Sprintf("%.1f", fd.Position.AltitudeAGL),
					"onGround", fd.Sensors.OnGround)
				currentInterval = newInterval
				ticker.Reset(currentInterval)
			}

			if len(pendingReports) > 0 {
				sent := 0
				for _, queued := range pendingReports {
					if _, _, err := a.Airspace.DoRequest("POST", "/api/v2/acars/position", queued); err != nil {
						break
					}
					sent++
				}
				if sent > 0 {
					pendingReports = pendingReports[sent:]
					posReportsSent.Add(context.Background(), int64(sent))
					slog.Info("sent queued position reports", "sent", sent, "remaining", len(pendingReports))
				}
			}

			if len(highResQueue) > 0 {
				posHighResDepth.Record(context.Background(), int64(len(highResQueue)))
				if _, _, err := a.Airspace.DoRequest("POST", "/api/v2/acars/position", highResQueue); err != nil {
					posReportsFailed.Add(context.Background(), int64(len(highResQueue)))
				} else {
					posReportsSent.Add(context.Background(), int64(len(highResQueue)))
					highResQueue = nil
				}
			}

			if !collecting {
				report := a.buildPositionReport(fd)
				_, _, err = a.Airspace.DoRequest("POST", "/api/v2/acars/position", report)
				if err != nil {
					consecutiveFailures++
					if len(pendingReports) < maxPendingReports {
						pendingReports = append(pendingReports, report)
						posReportsQueued.Add(context.Background(), 1)
					}
					if consecutiveFailures == 1 {
						slog.Warn("server connection lost, queuing position reports", "error", err)
					} else if consecutiveFailures%30 == 0 {
						slog.Warn("server still unreachable", "failures", consecutiveFailures, "queued", len(pendingReports))
					}
				} else {
					posReportsSent.Add(context.Background(), 1)
					if consecutiveFailures > 0 {
						slog.Info("server connection restored", "had_failures", consecutiveFailures, "queued_remaining", len(pendingReports))
					}
					consecutiveFailures = 0
				}
			}
		}
	}
}

func (a *App) flushPendingReports(pending []map[string]interface{}) {
	for i, report := range pending {
		if _, _, err := a.Airspace.DoRequest("POST", "/api/v2/acars/position", report); err != nil {
			slog.Warn("failed to flush queued report on flight end", "remaining", len(pending)-i, "error", err)
			posFlushTotal.Add(context.Background(), 1, metric.WithAttributes(attribute.Bool("success", false)))
			return
		}
	}
	if len(pending) > 0 {
		slog.Info("flushed all queued position reports", "count", len(pending))
		posFlushTotal.Add(context.Background(), 1, metric.WithAttributes(attribute.Bool("success", true)))
		posReportsSent.Add(context.Background(), int64(len(pending)))
	}
}
