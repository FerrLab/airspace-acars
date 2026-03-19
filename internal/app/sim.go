package app

import (
	"context"
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
	simTracer               = observability.Tracer("sim")
	simMeter                = observability.Meter("sim")
	simReconnectAttempts, _ = simMeter.Int64Counter("sim.reconnect_attempts",
		metric.WithDescription("Simulator reconnection attempts"))
	simStalenessDetected, _ = simMeter.Int64Counter("sim.staleness_detected",
		metric.WithDescription("Stale simulator connection events"))
)

const (
	stalenessThreshold  = 10 * time.Second
	reconnectBaseDelay  = 5 * time.Second
	reconnectMaxBackoff = 60 * time.Second
)

// ConnectSim connects to a flight simulator. simType can be "auto", "simconnect", or "xplane".
func (a *App) ConnectSim(simType string) (string, error) {
	_, span := simTracer.Start(context.Background(), "sim.connect",
		trace.WithAttributes(attribute.String("sim.type", simType)))
	defer span.End()

	a.simMu.Lock()

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

// SetSimPollRate adjusts the simulator adapter's data request frequency.
func (a *App) SetSimPollRate(d time.Duration) {
	a.simMu.Lock()
	connector := a.connector
	a.simMu.Unlock()

	type pollRateSetter interface {
		SetPollRate(time.Duration)
	}
	if pr, ok := connector.(pollRateSetter); ok {
		pr.SetPollRate(d)
	}
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
