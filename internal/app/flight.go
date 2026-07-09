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
	posOutboxEnqueued, _ = flightMeter.Int64Counter("position.outbox_enqueued",
		metric.WithDescription("Position reports persisted to the SQLite outbox"))
	posOutboxDepth, _    = flightMeter.Int64Histogram("position.outbox_depth",
		metric.WithDescription("Outbox row count sampled at each drain pass"))
	finishDrainDur, _    = flightMeter.Float64Histogram("flight.finish_drain_duration_sec",
		metric.WithDescription("Seconds from FinishFlight call to /api/v2/acars/finish success"))
	finishCanceledTotal, _ = flightMeter.Int64Counter("flight.finish_canceled_total",
		metric.WithDescription("Number of times CancelFinish was invoked"))
)

const (
	highResInterval      = 33 * time.Millisecond
	posIntervalCritical  = 500 * time.Millisecond
	posIntervalLow       = 1 * time.Second
	posIntervalHigh      = 2 * time.Second
	posIntervalStatic    = 60 * time.Second
	flareAltThreshold    = 50.0
	criticalAltThreshold = 200.0
	highAltThreshold     = 10_000.0
	maxHighResReports    = 3000
	maxBatchSize         = 250
	retryAttempts        = 4

	finishDrainTickEvery  = 1 * time.Second
	finishDrainBackoffMax = 60 * time.Second

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
	body, _, err := a.Airspace.DoRequest("GET", "/api/v2/acars/booking", nil)
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
func (a *App) StartFlight(callsign, departure, arrival, bookingID string) error {
	_, span := flightTracer.Start(context.Background(), "flight.start",
		trace.WithAttributes(
			attribute.String("flight.callsign", callsign),
			attribute.String("flight.departure", departure),
			attribute.String("flight.arrival", arrival),
			attribute.String("flight.booking_id", bookingID),
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

	_, status, err := a.Airspace.DoRequest("POST", "/api/v2/acars/start", payload)
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
	a.bookingID = bookingID
	a.startTime = time.Now()
	a.stopCh = make(chan struct{})

	go a.positionLoop(a.stopCh)

	slog.Info("flight started", "callsign", callsign, "dep", departure, "arr", arrival)
	a.UI.EmitEvent("flight-state", "active")
	if bookingID != "" {
		if n, err := a.DB.CountOutbox(bookingID); err == nil && n > 0 {
			slog.Info("resuming unsent positions from previous session", "booking_id", bookingID, "count", n)
			a.UI.EmitEvent("flight-outbox-resuming", map[string]interface{}{"pending": n})
		}
	}
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

	_, _, err := a.doRequestWithRetry("POST", "/api/v2/acars/stop", payload)
	if err != nil {
		slog.Warn("stop flight request failed after retries", "error", err)
	}

	a.endFlight()
	slog.Info("flight stopped/cancelled")
	return nil
}

// FinishFlight transitions to the "finishing" state and kicks off an async
// drain. Returns immediately. The frontend listens for flight-finish-*
// events to render progress and the terminal state.
func (a *App) FinishFlight() error {
	_, span := flightTracer.Start(context.Background(), "flight.finish")
	defer span.End()

	a.flightMu.Lock()

	span.SetAttributes(
		attribute.String("flight.callsign", a.callsign),
		attribute.Float64("flight.duration_sec", time.Since(a.startTime).Seconds()),
	)

	if a.state != "active" {
		a.flightMu.Unlock()
		err := fmt.Errorf("no active flight")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if elapsed := time.Since(a.startTime); elapsed < minFlightDuration {
		a.flightMu.Unlock()
		remaining := (minFlightDuration - elapsed).Round(time.Second)
		err := fmt.Errorf("flight too short to finish, please wait %s", remaining)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Snapshot what we need in the goroutine.
	bookingID := a.bookingID
	callsign := a.callsign
	departure := a.departure
	arrival := a.arrival

	// Stop the position loop (it will flush in-RAM highResQueue to outbox).
	if a.stopCh != nil {
		close(a.stopCh)
		a.stopCh = nil
	}
	a.state = "finishing"
	a.finishCancelCh = make(chan struct{})
	cancelCh := a.finishCancelCh

	a.flightMu.Unlock()

	a.UI.EmitEvent("flight-state", "finishing")
	slog.Info("flight finishing, draining outbox", "booking_id", bookingID, "callsign", callsign)

	go a.finishDrainLoop(bookingID, callsign, departure, arrival, cancelCh)
	return nil
}

// finishDrainLoop runs until the outbox for the booking is empty, then POSTs
// /api/v2/acars/finish. Emits flight-finish-progress events once per tick, and a
// terminal flight-finish-complete or flight-finish-failed event. Retries
// transient errors forever (exponential backoff capped at finishDrainBackoffMax).
// If cancelCh is closed, the loop exits and reverts state to "active".
func (a *App) finishDrainLoop(bookingID, callsign, departure, arrival string, cancelCh chan struct{}) {
	started := time.Now()
	backoff := 2 * time.Second

	for {
		select {
		case <-cancelCh:
			a.flightMu.Lock()
			a.state = "active"
			a.stopCh = make(chan struct{})
			a.flightMu.Unlock()
			go a.positionLoop(a.stopCh)
			a.UI.EmitEvent("flight-state", "active")
			slog.Info("flight finish cancelled by user", "booking_id", bookingID)
			return
		default:
		}

		count, err := a.DB.CountOutbox(bookingID)
		if err != nil {
			slog.Warn("outbox count failed during finish drain", "error", err)
			a.sleepOrCancel(backoff, cancelCh)
			backoff = minDuration(backoff*2, finishDrainBackoffMax)
			continue
		}

		if count == 0 {
			payload := map[string]interface{}{
				"callsign":  callsign,
				"departure": departure,
				"arrival":   arrival,
				"timestamp": time.Now().UnixMilli(),
			}
			body, status, err := a.doRequestWithRetry("POST", "/api/v2/acars/finish", payload)
			if err != nil || status >= 400 {
				msg := "server error"
				if err != nil {
					msg = err.Error()
				} else {
					var errResp map[string]interface{}
					_ = json.Unmarshal(body, &errResp)
					if m, ok := errResp["error"].(string); ok {
						msg = m
					}
				}
				a.flightMu.Lock()
				a.state = "active"
				a.stopCh = make(chan struct{})
				a.flightMu.Unlock()
				go a.positionLoop(a.stopCh)
				a.UI.EmitEvent("flight-state", "active")
				a.UI.EmitEvent("flight-finish-failed", map[string]interface{}{"reason": msg})
				slog.Warn("flight finish request failed", "error", msg)
				return
			}

			// Success.
			a.flightMu.Lock()
			a.endFlight()
			a.flightMu.Unlock()
			a.UI.EmitEvent("flight-finish-complete", map[string]interface{}{
				"duration_sec": time.Since(started).Seconds(),
			})
			finishDrainDur.Record(context.Background(), time.Since(started).Seconds())
			slog.Info("flight finished cleanly",
				"booking_id", bookingID,
				"callsign", callsign,
				"drain_sec", time.Since(started).Seconds())
			return
		}

		a.UI.EmitEvent("flight-finish-progress", map[string]interface{}{"pending": count})
		posOutboxDepth.Record(context.Background(), int64(count))

		sent, _, err := a.drainOutbox(bookingID, 4) // 4 batches = 1000 rows per pass
		if sent > 0 {
			posReportsSent.Add(context.Background(), int64(sent))
			backoff = 2 * time.Second
		}
		if err != nil {
			slog.Warn("finish drain batch failed, will retry", "error", err, "remaining", count-sent)
			a.sleepOrCancel(backoff, cancelCh)
			backoff = minDuration(backoff*2, finishDrainBackoffMax)
			continue
		}

		a.sleepOrCancel(finishDrainTickEvery, cancelCh)
	}
}

func (a *App) sleepOrCancel(d time.Duration, cancelCh chan struct{}) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-cancelCh:
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// CancelFinish aborts an in-progress finish drain and returns to "active".
// The outbox is left intact; the user can retry FinishFlight later.
func (a *App) CancelFinish() error {
	a.flightMu.Lock()
	if a.state != "finishing" {
		a.flightMu.Unlock()
		return fmt.Errorf("no finish in progress")
	}
	ch := a.finishCancelCh
	a.finishCancelCh = nil
	a.flightMu.Unlock()

	if ch != nil {
		close(ch)
	}
	finishCanceledTotal.Add(context.Background(), 1)
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
	a.bookingID = ""
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

// sendPositionBatches POSTs reports to /api/v2/acars/position in chunks of at
// most maxBatchSize. Returns the number sent successfully before returning.
// On error, returns (sent, err) and leaves the unsent suffix to the caller.
func (a *App) sendPositionBatches(reports []map[string]interface{}) (int, error) {
	sent := 0
	for sent < len(reports) {
		end := sent + maxBatchSize
		if end > len(reports) {
			end = len(reports)
		}
		batch := reports[sent:end]
		if _, _, err := a.Airspace.DoRequest("POST", "/api/v2/acars/position", batch); err != nil {
			return sent, err
		}
		sent = end
	}
	return sent, nil
}

// drainOutbox pulls up to maxBatches batches of maxBatchSize from the outbox
// for the given booking and POSTs each. Successfully shipped rows are deleted
// from the DB. Returns (sent, remaining, err); on error the unsent rows stay.
func (a *App) drainOutbox(bookingID string, maxBatches int) (int, int, error) {
	if bookingID == "" {
		return 0, 0, nil
	}
	sent := 0
	for i := 0; i < maxBatches; i++ {
		ids, payloads, err := a.DB.PeekOutboxBatch(bookingID, maxBatchSize)
		if err != nil {
			remaining, _ := a.DB.CountOutbox(bookingID)
			return sent, remaining, err
		}
		if len(ids) == 0 {
			return sent, 0, nil
		}

		goodIDs := make([]int64, 0, len(ids))
		goodReports := make([]map[string]interface{}, 0, len(payloads))
		for j, raw := range payloads {
			var report map[string]interface{}
			if err := json.Unmarshal(raw, &report); err != nil {
				// Poison row — delete it inline so we don't loop forever.
				slog.Warn("outbox: dropping unparsable payload", "id", ids[j], "error", err)
				_ = a.DB.DeleteOutboxBatch([]int64{ids[j]})
				continue
			}
			goodIDs = append(goodIDs, ids[j])
			goodReports = append(goodReports, report)
		}

		if len(goodReports) == 0 {
			// All rows in this batch were poison and are already deleted; keep looping.
			continue
		}

		if _, _, err := a.Airspace.DoRequest("POST", "/api/v2/acars/position", goodReports); err != nil {
			remaining, _ := a.DB.CountOutbox(bookingID)
			return sent, remaining, err
		}
		if err := a.DB.DeleteOutboxBatch(goodIDs); err != nil {
			slog.Warn("outbox: delete after send failed", "count", len(goodIDs), "error", err)
			remaining, _ := a.DB.CountOutbox(bookingID)
			return sent, remaining, err
		}
		sent += len(goodIDs)
	}
	remaining, _ := a.DB.CountOutbox(bookingID)
	return sent, remaining, nil
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

	var highResQueue []map[string]interface{}
	var consecutiveFailures int
	collecting := false
	var touchdownGrace time.Time

	a.flightMu.Lock()
	bookingID := a.bookingID
	a.flightMu.Unlock()

	slog.Info("position loop started", "interval", intervalName(currentInterval), "booking_id", bookingID)

	persistQueue := func(q []map[string]interface{}, reason string) {
		if bookingID == "" || len(q) == 0 {
			return
		}
		for _, report := range q {
			raw, err := json.Marshal(report)
			if err != nil {
				slog.Warn("position: marshal for outbox failed", "error", err, "reason", reason)
				continue
			}
			if err := a.DB.EnqueuePosition(bookingID, raw); err != nil {
				slog.Warn("position: enqueue to outbox failed", "error", err, "reason", reason)
				continue
			}
			posReportsQueued.Add(context.Background(), 1)
			posOutboxEnqueued.Add(context.Background(), 1)
		}
	}

	for {
		select {
		case <-stopCh:
			slog.Info("position loop stopping",
				"collecting", collecting,
				"highResQueued", len(highResQueue))
			persistQueue(highResQueue, "loop-stop")
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

			posChanged := fd.Position.Latitude != lastLat || fd.Position.Longitude != lastLng
			if posChanged {
				lastLat = fd.Position.Latitude
				lastLng = fd.Position.Longitude
				lastChanged = time.Now()
			}

			// Flare zone detection (unchanged).
			shouldCollect := false
			if !fd.Sensors.OnGround && fd.Position.AltitudeAGL < flareAltThreshold {
				shouldCollect = true
				touchdownGrace = time.Time{}
			} else if collecting && fd.Sensors.OnGround {
				if touchdownGrace.IsZero() {
					touchdownGrace = time.Now()
					slog.Info("touchdown detected, continuing high-res collection",
						"agl", fd.Position.AltitudeAGL, "gs", fd.Attitude.GS)
				}
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
					"queued", len(highResQueue))
				touchdownGrace = time.Time{}
			}

			// Reporting interval selection.
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

			// Drain one batch from the outbox (non-blocking).
			if bookingID != "" {
				if sent, remaining, derr := a.drainOutbox(bookingID, 1); derr != nil {
					if consecutiveFailures%30 == 0 {
						slog.Warn("outbox drain failed", "error", derr, "remaining", remaining)
					}
				} else if sent > 0 {
					posReportsSent.Add(context.Background(), int64(sent))
					slog.Info("outbox drained", "sent", sent, "remaining", remaining)
				}
			}

			// High-res queue drain — batched; on failure persist to outbox.
			if len(highResQueue) > 0 {
				posHighResDepth.Record(context.Background(), int64(len(highResQueue)))
				shipped, err := a.sendPositionBatches(highResQueue)
				if shipped > 0 {
					posReportsSent.Add(context.Background(), int64(shipped))
				}
				if err != nil {
					unsent := highResQueue[shipped:]
					posReportsFailed.Add(context.Background(), int64(len(unsent)))
					persistQueue(unsent, "highres-drain-failed")
					highResQueue = nil
				} else {
					highResQueue = nil
				}
			}

			// Normal single-report send (only when not in high-res collection mode).
			if !collecting {
				report := a.buildPositionReport(fd)
				_, _, err = a.Airspace.DoRequest("POST", "/api/v2/acars/position", report)
				if err != nil {
					consecutiveFailures++
					if bookingID != "" {
						raw, mErr := json.Marshal(report)
						if mErr == nil {
							if eErr := a.DB.EnqueuePosition(bookingID, raw); eErr == nil {
								posReportsQueued.Add(context.Background(), 1)
								posOutboxEnqueued.Add(context.Background(), 1)
							}
						}
					}
					if consecutiveFailures == 1 {
						slog.Warn("server connection lost, queuing position reports", "error", err)
					} else if consecutiveFailures%30 == 0 {
						slog.Warn("server still unreachable", "failures", consecutiveFailures)
					}
				} else {
					posReportsSent.Add(context.Background(), 1)
					if consecutiveFailures > 0 {
						slog.Info("server connection restored", "had_failures", consecutiveFailures)
					}
					consecutiveFailures = 0
				}
			}
		}
	}
}
