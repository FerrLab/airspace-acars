package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var (
	simTracer               = observability.Tracer("sim")
	simMeter                = observability.Meter("sim")
	simReconnectAttempts, _ = simMeter.Int64Counter("sim.reconnect_attempts",
		metric.WithDescription("Simulator reconnection attempts"))
	simStalenessDetected, _ = simMeter.Int64Counter("sim.staleness_detected",
		metric.WithDescription("Stale simulator connection events"))
)

const (
	stalenessThreshold   = 10 * time.Second
	reconnectBaseDelay   = 5 * time.Second
	reconnectMaxBackoff  = 60 * time.Second
	autoConnectInterval  = 30 * time.Second
)

// ConnectSim connects to a flight simulator. simType can be "auto", "simconnect", or "xplane".
func (a *App) ConnectSim(simType string) (string, error) {
	_, span := simTracer.Start(context.Background(), "sim.connect",
		trace.WithAttributes(attribute.String("sim.type", simType)))
	defer span.End()

	a.simMu.Lock()
	a.userDisconnected = false

	if a.connector != nil {
		a.stopDataStreamLocked()
		a.connector.Disconnect()
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
			err := fmt.Errorf("SimConnect not available on this platform")
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return "", err
		}
	default: // "auto"
		sc := a.NewSimConnectAdapter()
		if sc != nil {
			if err := sc.Connect(); err == nil {
				connector = sc
				connected = true
			} else {
				slog.Info("SimConnect not available, trying X-Plane", "error", err)
			}
		}
		if connector == nil {
			connector = a.NewXPlaneAdapter("127.0.0.1", 49000)
		}
	}

	span.SetAttributes(attribute.String("sim.adapter", connector.Name()))

	if !connected {
		if err := connector.Connect(); err != nil {
			a.simMu.Unlock()
			err = fmt.Errorf("connect to %s: %w", connector.Name(), err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return "", err
		}
	}

	a.connector = connector
	a.simActive = false
	a.adapterName = connector.Name()
	a.reconnectAttempts = 0
	a.lastReconnectAt = time.Time{}
	slog.Info("adapter opened, waiting for data", "adapter", connector.Name())

	a.startDataStreamLocked()
	a.simMu.Unlock()

	// Wait up to 3 seconds for actual simulator data
	deadline := time.After(3 * time.Second)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			a.DisconnectSim()
			err := fmt.Errorf("no data received from %s — is the simulator running?", connector.Name())
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return "", err
		case <-tick.C:
			a.simMu.Lock()
			active := a.simActive
			a.simMu.Unlock()
			if active {
				slog.Info("connected to simulator", "adapter", connector.Name())
				return connector.Name(), nil
			}
		}
	}
}

// DisconnectSim disconnects from the simulator.
func (a *App) DisconnectSim() {
	_, span := simTracer.Start(context.Background(), "sim.disconnect")
	defer span.End()

	a.simMu.Lock()
	defer a.simMu.Unlock()

	span.SetAttributes(attribute.String("sim.adapter", a.adapterName))

	a.stopDataStreamLocked()

	if a.connector != nil {
		a.connector.Disconnect()
		a.connector = nil
	}

	a.simActive = false
	a.adapterName = ""
	a.reconnectAttempts = 0
	a.lastReconnectAt = time.Time{}
	a.userDisconnected = true
	a.UI.EmitEvent("connection-state", "")
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
	go a.dataStreamLoop()
}

func (a *App) stopDataStreamLocked() {
	if !a.streaming {
		return
	}
	a.streaming = false
	close(a.streamStopCh)
}

func (a *App) dataStreamLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.streamStopCh:
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
				a.simActive = true
				a.reconnectAttempts = 0
				a.lastReconnectAt = time.Time{}
				a.simMu.Unlock()
				a.UI.EmitEvent("connection-state", connector.Name())
				slog.Info("simulator data received", "adapter", connector.Name())
			}

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

			// Staleness check
			if wasActive && !connector.LastReceived().IsZero() &&
				time.Since(connector.LastReceived()) > stalenessThreshold {

				a.simMu.Lock()
				backoff := time.Duration(1<<uint(a.reconnectAttempts)) * reconnectBaseDelay
				if backoff > reconnectMaxBackoff {
					backoff = reconnectMaxBackoff
				}
				if time.Since(a.lastReconnectAt) < backoff {
					a.simMu.Unlock()
					continue
				}

				a.lastReconnectAt = time.Now()
				a.simActive = false
				attempt := a.reconnectAttempts + 1
				a.simMu.Unlock()

				a.UI.EmitEvent("connection-state", "")
				simStalenessDetected.Add(context.Background(), 1,
					metric.WithAttributes(attribute.String("adapter", adapterName)))
				slog.Warn("simulator connection stale, reconnecting",
					"adapter", adapterName,
					"lastData", connector.LastReceived(),
					"attempt", attempt)

				err := a.reconnectSim()
				simReconnectAttempts.Add(context.Background(), 1,
					metric.WithAttributes(
						attribute.String("adapter", adapterName),
						attribute.Bool("success", err == nil)))
				if err != nil {
					a.simMu.Lock()
					a.reconnectAttempts++
					attempts := a.reconnectAttempts
					a.simMu.Unlock()
					slog.Error("reconnect failed",
						"adapter", adapterName,
						"attempt", attempts,
						"error", err)
				} else {
					slog.Info("reconnected successfully", "adapter", adapterName)
				}
			}
		}
	}
}

// AutoConnectLoop tries to connect to the simulator every 30 seconds when not connected.
// It should be called as a goroutine. The first attempt happens immediately.
func (a *App) AutoConnectLoop() {
	settings := a.GetSettings()
	if adapter, err := a.ConnectSim(settings.SimType); err != nil {
		slog.Debug("auto-connect: initial attempt failed", "error", err)
	} else {
		slog.Info("auto-connected to simulator", "adapter", adapter)
	}

	ticker := time.NewTicker(autoConnectInterval)
	defer ticker.Stop()

	for range ticker.C {
		a.simMu.Lock()
		skip := a.simActive || a.userDisconnected
		a.simMu.Unlock()
		if skip {
			continue
		}
		settings := a.GetSettings()
		if adapter, err := a.ConnectSim(settings.SimType); err != nil {
			slog.Debug("auto-connect: attempt failed", "error", err)
		} else {
			slog.Info("auto-connected to simulator", "adapter", adapter)
		}
	}
}

func (a *App) reconnectSim() error {
	a.simMu.Lock()
	old := a.connector
	a.connector = nil
	a.simActive = false
	name := a.adapterName
	a.simMu.Unlock()

	_, span := simTracer.Start(context.Background(), "sim.reconnect",
		trace.WithAttributes(attribute.String("sim.adapter", name)))
	defer span.End()

	if old != nil {
		old.Disconnect()
	}

	var connector domain.SimConnector
	switch name {
	case "SimConnect":
		connector = a.NewSimConnectAdapter()
		if connector == nil {
			err := fmt.Errorf("SimConnect not available")
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	case "X-Plane":
		connector = a.NewXPlaneAdapter("127.0.0.1", 49000)
	default:
		err := fmt.Errorf("unknown adapter: %s", name)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if err := connector.Connect(); err != nil {
		err = fmt.Errorf("reconnect %s: %w", name, err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	a.simMu.Lock()
	a.connector = connector
	a.simMu.Unlock()
	return nil
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
	body, _, err := a.Airspace.DoRequest("GET", "/api/acars/booking", nil)
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

