package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

const (
	// stalenessThreshold is how long a connection may go without hearing from
	// the simulator before it is given up on and replaced. A stutter or a
	// loading screen shorter than this is ridden out on the same connection.
	stalenessThreshold = 10 * time.Second

	// autoConnectInterval is how often the auto-connect loop checks on the
	// simulator and, when nothing is connected, tries again. A simulator that
	// drops out mid-flight — a long stutter, a loading screen — is picked back
	// up as soon as it answers, not up to half a minute later.
	autoConnectInterval = time.Second

	// simDataWait is how long a freshly opened adapter is given to send
	// something before it is treated as not there. A UDP socket to X-Plane
	// opens whether or not X-Plane is running, so data is the only proof.
	simDataWait = 3 * time.Second

	// simRetryReminder is how often a simulator that is still not answering
	// is mentioned in the log again, once the first failure has been.
	simRetryReminder = 5 * time.Minute
)

// errNoSimConnect is returned when SimConnect is asked for on a platform that
// does not have it.
var errNoSimConnect = errors.New("SimConnect not available on this platform")

// ConnectSim connects to a flight simulator at the pilot's request. simType can
// be "auto", "simconnect", or "xplane". It also ends a manual disconnect: the
// auto-connect loop picks up again whether or not this attempt succeeds.
func (a *App) ConnectSim(simType string) (string, error) {
	_, span := observability.Start(context.Background(), "sim.connect",
		"sim.type", simType)
	defer span.Finish()

	// An auto-connect attempt already under way is let finish, rather than
	// having its adapter closed while it waits.
	a.connectMu.Lock()
	defer a.connectMu.Unlock()

	a.simMu.Lock()
	a.userDisconnected = false
	// The pilot asking is a fresh start, so this attempt is logged in full
	// even in the middle of a streak of failed auto-connects.
	a.simWaitFailures = 0
	a.simMu.Unlock()

	adapter, err := a.connectSim(simType)
	if adapter != "" {
		span.Set("sim.adapter", adapter)
	}
	if errors.Is(err, errNoSimConnect) {
		span.Fail(err)
		return "", err
	}
	if err != nil {
		span.Expected(err)
		return "", err
	}
	return adapter, nil
}

// connectSim opens an adapter for simType, replacing whatever connection there
// was, and waits for the simulator to answer through it. It is the whole of one
// attempt, for the pilot's Connect button and the auto-connect loop alike, and
// it leaves the pilot's choice to connect or disconnect alone: a failed attempt
// is only a failed attempt, and the loop tries again.
//
// It returns the adapter it tried, when it got as far as choosing one, even on
// failure. Callers hold connectMu.
func (a *App) connectSim(simType string) (string, error) {
	a.simMu.Lock()

	// Once a streak of failures has been explained in the log, the attempts
	// that follow keep their steps to Debug: at one a second they would
	// otherwise bury everything else in it.
	level := slog.LevelInfo
	if a.simWaitFailures > 0 {
		level = slog.LevelDebug
	}

	if a.closeSimLocked() {
		a.UI.EmitEvent("connection-state", "")
	}

	var connector domain.SimConnector
	connected := false

	switch simType {
	case "xplane":
		connector = a.NewXPlaneAdapter("127.0.0.1", 49000)
	case "simconnect":
		connector = a.NewSimConnectAdapter()
		if connector == nil {
			a.simMu.Unlock()
			a.noteSimAttemptFailed("SimConnect", errNoSimConnect)
			return "", errNoSimConnect
		}
	default: // "auto"
		sc := a.NewSimConnectAdapter()
		if sc != nil {
			if err := sc.Connect(); err == nil {
				connector = sc
				connected = true
			} else {
				slog.Log(context.Background(), level,
					"SimConnect not available, trying X-Plane", "error", err)
			}
		}
		if connector == nil {
			connector = a.NewXPlaneAdapter("127.0.0.1", 49000)
		}
	}

	if !connected {
		if err := connector.Connect(); err != nil {
			a.simMu.Unlock()
			err = fmt.Errorf("connect to %s: %w", connector.Name(), err)
			a.noteSimAttemptFailed(connector.Name(), err)
			return connector.Name(), err
		}
	}

	a.connector = connector
	a.adapterName = connector.Name()
	slog.Log(context.Background(), level,
		"adapter opened, waiting for data", "adapter", connector.Name())

	a.startDataStreamLocked()
	a.simMu.Unlock()

	// Wait for actual simulator data: opening an adapter proves nothing.
	deadline := time.After(simDataWait)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			a.dropSim(connector)
			err := fmt.Errorf("no data received from %s — is the simulator running?", connector.Name())
			a.noteSimAttemptFailed(connector.Name(), err)
			return connector.Name(), err
		case <-tick.C:
			a.simMu.Lock()
			active := a.simActive && a.connector == connector
			missed := a.simWaitFailures
			if active {
				a.simWaitFailures = 0
			}
			a.simMu.Unlock()
			if active {
				slog.Info("connected to simulator",
					"adapter", connector.Name(), "after_failed_attempts", missed)
				return connector.Name(), nil
			}
		}
	}
}

// noteSimAttemptFailed reports an attempt to reach the simulator that came to
// nothing: an adapter that would not open, or one that opened and never heard
// back.
//
// The auto-connect loop tries again every second, so this cannot log every
// attempt. It also cannot stay at Debug, which is where it was: that is not in
// a pilot's log, so a log from someone whose simulator is not running read
// exactly like a log from someone whose ACARS is broken, and the only line that
// explained it was the one nobody had.
//
// So: the first one, and then a reminder every few minutes saying how many
// attempts it stands for. The reminder goes by the clock rather than by a
// count, because how long an attempt takes depends on why it failed.
func (a *App) noteSimAttemptFailed(adapter string, err error) {
	a.simMu.Lock()
	a.simWaitFailures++
	n := a.simWaitFailures
	remind := n > 1 && time.Since(a.simWaitNotedAt) >= simRetryReminder
	if n == 1 || remind {
		a.simWaitNotedAt = time.Now()
	}
	a.simMu.Unlock()

	switch {
	case n == 1:
		slog.Warn("simulator not answering; will keep trying",
			"adapter", adapter,
			"error", err,
			"retry_every", autoConnectInterval.String())
	case remind:
		slog.Warn("simulator still not answering",
			"adapter", adapter, "attempts", n, "error", err)
	}
}

// DisconnectSim disconnects from the simulator at the pilot's request, and
// stands the auto-connect loop down until they connect again.
func (a *App) DisconnectSim() {
	_, span := observability.Start(context.Background(), "sim.disconnect")
	defer span.Finish()

	a.simMu.Lock()
	defer a.simMu.Unlock()

	span.Set("sim.adapter", a.adapterName)

	a.closeSimLocked()
	a.userDisconnected = true
	a.UI.EmitEvent("connection-state", "")
}

// dropSim closes connector, if it is still the one in use, without it being
// the pilot's doing: the auto-connect loop finds nothing connected and opens a
// fresh adapter within a second. It is how a connection attempt that never
// heard back and a connection that has gone quiet both end.
func (a *App) dropSim(connector domain.SimConnector) {
	a.simMu.Lock()
	defer a.simMu.Unlock()

	if a.connector != connector {
		return
	}
	if a.closeSimLocked() {
		a.UI.EmitEvent("connection-state", "")
	}
}

// closeSimLocked closes the adapter and stops its data stream, and reports
// whether the connection was live, for the caller to tell the UI. Every way a
// connection ends comes through here. Callers hold simMu.
func (a *App) closeSimLocked() bool {
	wasActive := a.simActive

	a.stopDataStreamLocked()
	if a.connector != nil {
		a.connector.Disconnect()
		a.connector = nil
	}
	a.simActive = false
	a.adapterName = ""

	// The next adapter starts without the aircraft profile installed. Unless
	// the profile is forgotten here, the next connection finds the same
	// aircraft, takes it for no change, and never hands the profile over.
	a.clearAircraftProfile()

	return wasActive
}

// IsConnected returns whether the simulator is actively sending data.
func (a *App) IsConnected() bool {
	a.simMu.Lock()
	defer a.simMu.Unlock()
	return a.simActive
}

// ConnectedAdapter returns the name of the active simulator adapter, or empty string.
func (a *App) ConnectedAdapter() string {
	a.simMu.Lock()
	defer a.simMu.Unlock()
	if a.simActive && a.connector != nil {
		return a.connector.Name()
	}
	return ""
}

// GetFlightDataNow returns a one-shot read of the current flight data.
func (a *App) GetFlightDataNow() (*domain.FlightData, error) {
	a.simMu.Lock()
	connector := a.connector
	a.simMu.Unlock()

	if connector == nil {
		return nil, fmt.Errorf("no simulator connected")
	}
	return connector.GetFlightData()
}

func (a *App) startDataStreamLocked() {
	if a.streaming {
		return
	}
	a.streaming = true
	a.streamStopCh = make(chan struct{})
	go a.dataStreamLoop(a.streamStopCh)
}

func (a *App) stopDataStreamLocked() {
	if !a.streaming {
		return
	}
	a.streaming = false
	close(a.streamStopCh)
}

// dataStreamLoop reads the simulator once a second until stop is closed.
//
// It is handed its stop channel rather than reading a.streamStopCh, which the
// next connection replaces: a loop stopped while it was mid-tick would
// otherwise come back around to its successor's channel and run on beside it.
func (a *App) dataStreamLoop(stop <-chan struct{}) {
	defer observability.Recover()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			a.simMu.Lock()
			connector := a.connector
			recording := a.recording
			wasActive := a.simActive
			adapterName := a.adapterName
			a.simMu.Unlock()

			if connector == nil {
				continue
			}

			data, err := connector.GetFlightData()
			if err != nil {
				if wasActive {
					a.simMu.Lock()
					a.simActive = false
					a.simMu.Unlock()
					a.UI.EmitEvent("connection-state", "")
					slog.Warn("simulator data lost", "error", err)
				}
				continue
			}

			if !wasActive {
				a.simMu.Lock()
				if a.connector != connector {
					// Replaced while this tick was reading it; the new
					// connection has a loop of its own.
					a.simMu.Unlock()
					return
				}
				a.simActive = true
				a.simMu.Unlock()
				a.UI.EmitEvent("connection-state", connector.Name())
				slog.Info("simulator data received", "adapter", connector.Name())
			}

			// Resolve the aircraft profile before the data is published, so
			// the adapter is already collecting the variables it asks for.
			a.refreshAircraftProfile(connector)

			a.UI.EmitEvent("flight-data", data)

			if recording {
				if err := a.DB.SaveFlightData(data); err != nil {
					slog.Error("failed to insert flight data", "error", err)
					continue
				}
				a.simMu.Lock()
				a.dataCount++
				a.simMu.Unlock()
			}

			// Auto-flight detection
			a.checkAutoFlight(data)

			// Staleness check. A connection that has gone quiet is dropped,
			// and the auto-connect loop opens a fresh one within a second.
			// SimConnect goes on returning its last reading after the
			// simulator stops answering, so this is how that is noticed.
			if wasActive && !connector.LastReceived().IsZero() &&
				time.Since(connector.LastReceived()) > stalenessThreshold {
				observability.Count("sim.staleness_detected", "adapter", adapterName)
				slog.Warn("simulator connection stale, reconnecting",
					"adapter", adapterName,
					"lastData", connector.LastReceived())
				a.dropSim(connector)
				return
			}
		}
	}
}

// AutoConnectLoop keeps the simulator connected: every second, if nothing is,
// it tries again, until the pilot disconnects by hand. It should be called as a
// goroutine. The first attempt happens immediately.
func (a *App) AutoConnectLoop() {
	defer observability.Recover()

	ticker := time.NewTicker(autoConnectInterval)
	defer ticker.Stop()

	for {
		a.autoConnect()
		<-ticker.C
	}
}

// autoConnect is one turn of the auto-connect loop: an attempt, when nothing
// is connected, the pilot has not disconnected, and no connection is still
// riding out a stall.
//
// Its attempts are not traced one by one. At one a second, a span each would
// ship a steady stream of sampled transactions and fill the breadcrumb buffer
// with "is the simulator running?"; noteSimAttemptFailed keeps the log of the
// streak instead, and the pilot's own Connect is still traced.
func (a *App) autoConnect() {
	// A connection the pilot started is still waiting for data; leave it be.
	if !a.connectMu.TryLock() {
		return
	}
	defer a.connectMu.Unlock()

	a.simMu.Lock()
	skip := a.simActive || a.userDisconnected
	connector := a.connector
	a.simMu.Unlock()
	if skip {
		return
	}

	// A connection that has only just gone quiet — X-Plane stops sending in a
	// long stutter — is given as long to come back as a live one would be. Its
	// adapter is still subscribed and listening, so it picks the simulator up
	// the moment it answers; replacing it sooner would only pile a new set of
	// subscriptions onto a simulator that is not reading them.
	if connector != nil {
		if last := connector.LastReceived(); !last.IsZero() && time.Since(last) < stalenessThreshold {
			return
		}
	}

	adapter, err := a.connectSim(a.GetSettings().SimType)
	if err != nil {
		slog.Debug("auto-connect: attempt failed", "error", err)
		return
	}
	slog.Info("auto-connected to simulator", "adapter", adapter)
}

// checkAutoFlight evaluates conditions for auto-start.
// Called once per tick from dataStreamLoop (single goroutine, no mutex needed for armed flags).
func (a *App) checkAutoFlight(data *domain.FlightData) {
	settings := a.GetSettings()
	if settings.LocalMode {
		return
	}

	a.flightMu.Lock()
	flightState := a.state
	a.flightMu.Unlock()

	// --- Auto-start: beacon on, engine(s) running, on ground, stationary, flight idle ---
	if settings.AutoStartFlight && flightState == "idle" {
		anyEngineRunning := false
		for _, e := range data.Engines {
			if e.Running {
				anyEngineRunning = true
				break
			}
		}
		startConditions := data.Lights.Beacon && anyEngineRunning &&
			data.Sensors.OnGround && data.Attitude.GS < 1.0

		if startConditions && !a.autoStartArmed {
			a.autoStartArmed = true
			go a.tryAutoStartFlight()
		} else if !startConditions {
			a.autoStartArmed = false
		}
	}
}

// tryAutoStartFlight fetches the booking and starts the flight automatically.
func (a *App) tryAutoStartFlight() {
	defer observability.Recover()

	body, status, err := a.Airspace.DoRequest("GET", "/api/v2/acars/booking", nil)
	if err == nil {
		err = domain.NewStatusError("GET", "/api/v2/acars/booking", status, body)
	}
	if err != nil {
		slog.Debug("auto-start: failed to fetch booking", "error", err)
		return
	}
	var booking map[string]interface{}
	if err := json.Unmarshal(body, &booking); err != nil {
		slog.Debug("auto-start: failed to parse booking", "error", err)
		return
	}

	callsign, _ := booking["callsign"].(string)
	if callsign == "" {
		if fn, ok := booking["flight_number"].(string); ok {
			callsign = fn
		}
	}
	if callsign == "" {
		slog.Debug("auto-start: no booking available")
		return
	}

	var departure, arrival string
	if dep, ok := booking["departure_airport"].(map[string]interface{}); ok {
		departure, _ = dep["icao"].(string)
	}
	if alt, ok := booking["alternate_airport"].(map[string]interface{}); ok {
		arrival, _ = alt["icao"].(string)
	}
	if arrival == "" {
		if arr, ok := booking["arrival_airport"].(map[string]interface{}); ok {
			arrival, _ = arr["icao"].(string)
		}
	}

	var bookingID string
	switch v := booking["id"].(type) {
	case string:
		bookingID = v
	case float64:
		bookingID = strconv.FormatInt(int64(v), 10)
	}

	a.UI.EmitEvent("auto-flight-start", callsign)
	if err := a.StartFlight(callsign, departure, arrival, bookingID); err != nil {
		slog.Warn("auto-start: failed to start flight", "error", err)
	} else {
		slog.Info("auto-start: flight started", "callsign", callsign, "dep", departure, "arr", arrival)
	}
}
