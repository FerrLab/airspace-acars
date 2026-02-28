package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type FlightDataService struct {
	db           *sql.DB
	app          *application.App
	connector    SimConnector
	mu           sync.Mutex
	recording    bool
	startTime    time.Time
	dataCount    int
	streaming    bool
	streamStopCh chan struct{}
	simActive    bool
	lastSimType  string // remembered sim type for auto-reconnect
}

func NewFlightDataService(db *sql.DB) *FlightDataService {
	return &FlightDataService{
		db: db,
	}
}

func (f *FlightDataService) setApp(app *application.App) {
	f.app = app
}

func (f *FlightDataService) ConnectSim(simType string) (string, error) {
	f.mu.Lock()

	if f.connector != nil {
		f.stopDataStreamLocked()
		f.connector.Disconnect()
	}

	var connector SimConnector
	connected := false

	switch simType {
	case "xplane":
		connector = NewXPlaneAdapter("127.0.0.1", 49000)
	case "simconnect":
		connector = NewSimConnectAdapter()
		if connector == nil {
			f.mu.Unlock()
			return "", fmt.Errorf("SimConnect not available on this platform")
		}
	default: // "auto"
		sc := NewSimConnectAdapter()
		if sc != nil {
			if err := sc.Connect(); err == nil {
				connector = sc
				connected = true
			} else {
				slog.Info("SimConnect not available, trying X-Plane", "error", err)
			}
		}
		if connector == nil {
			connector = NewXPlaneAdapter("127.0.0.1", 49000)
		}
	}

	if !connected {
		if err := connector.Connect(); err != nil {
			f.mu.Unlock()
			return "", fmt.Errorf("connect to %s: %w", connector.Name(), err)
		}
	}

	f.connector = connector
	f.simActive = false
	f.lastSimType = simType
	slog.Info("adapter opened, waiting for data", "adapter", connector.Name())

	f.startDataStreamLocked()
	f.mu.Unlock()

	// Wait up to 3 seconds for actual simulator data
	deadline := time.After(3 * time.Second)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			f.DisconnectSim()
			return "", fmt.Errorf("no data received from %s — is the simulator running?", connector.Name())
		case <-tick.C:
			f.mu.Lock()
			active := f.simActive
			f.mu.Unlock()
			if active {
				slog.Info("connected to simulator", "adapter", connector.Name())
				return connector.Name(), nil
			}
		}
	}
}

func (f *FlightDataService) DisconnectSim() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.stopDataStreamLocked()

	if f.connector != nil {
		f.connector.Disconnect()
		f.connector = nil
	}

	f.simActive = false
	if f.app != nil {
		f.app.Event.Emit("connection-state", "")
	}
}

func (f *FlightDataService) IsConnected() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.simActive
}

func (f *FlightDataService) ConnectedAdapter() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.simActive && f.connector != nil {
		return f.connector.Name()
	}
	return ""
}

func (f *FlightDataService) StartRecording() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.connector == nil {
		return fmt.Errorf("no simulator connected")
	}
	if f.recording {
		return fmt.Errorf("already recording")
	}

	f.recording = true
	f.startTime = time.Now()
	f.dataCount = 0

	if f.app != nil {
		f.app.Event.Emit("recording-state", true)
	}
	return nil
}

func (f *FlightDataService) StopRecording() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.recording {
		return
	}

	f.recording = false

	if f.app != nil {
		f.app.Event.Emit("recording-state", false)
	}
}

func (f *FlightDataService) IsRecording() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recording
}

func (f *FlightDataService) GetRecordingInfo() map[string]interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	duration := 0.0
	if f.recording {
		duration = time.Since(f.startTime).Seconds()
	}

	return map[string]interface{}{
		"recording": f.recording,
		"duration":  duration,
		"dataCount": f.dataCount,
	}
}

func (f *FlightDataService) ExportCSV(filePath string) error {
	rows, err := f.db.Query(`SELECT timestamp, data FROM flight_data ORDER BY id`)
	if err != nil {
		return fmt.Errorf("query data: %w", err)
	}
	defer rows.Close()

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	w := csv.NewWriter(file)
	defer w.Flush()

	w.Write([]string{
		"timestamp",
		"latitude", "longitude", "altitude", "altitudeAGL",
		"pitch", "roll", "headingTrue", "headingMag", "vs", "ias", "tas", "gs",
		"eng1Running", "eng1N1", "eng1N2", "eng1Throttle",
		"eng2Running", "eng2N1", "eng2N2", "eng2Throttle",
		"onGround", "stallWarning", "overspeedWarning",
		"com1", "com2", "nav1", "nav2", "xpdrCode",
		"apMaster", "apHeading", "apAltitude", "apVS", "apSpeed",
		"altimeterInHg",
		"beacon", "strobe", "landing",
		"elevator", "aileron", "rudder", "flaps", "spoilers", "gearDown",
	})

	ff := func(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }
	fb := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}

	for rows.Next() {
		var ts, dataJSON string
		if err := rows.Scan(&ts, &dataJSON); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}

		var d FlightData
		if err := json.Unmarshal([]byte(dataJSON), &d); err != nil {
			return fmt.Errorf("unmarshal row: %w", err)
		}

		w.Write([]string{
			ts,
			ff(d.Position.Latitude), ff(d.Position.Longitude), ff(d.Position.Altitude), ff(d.Position.AltitudeAGL),
			ff(d.Attitude.Pitch), ff(d.Attitude.Roll), ff(d.Attitude.HeadingTrue), ff(d.Attitude.HeadingMag),
			ff(d.Attitude.VS), ff(d.Attitude.IAS), ff(d.Attitude.TAS), ff(d.Attitude.GS),
			fb(d.Engines[0].Running), ff(d.Engines[0].N1), ff(d.Engines[0].N2), ff(d.Engines[0].ThrottlePos),
			fb(d.Engines[1].Running), ff(d.Engines[1].N1), ff(d.Engines[1].N2), ff(d.Engines[1].ThrottlePos),
			fb(d.Sensors.OnGround), fb(d.Sensors.StallWarning), fb(d.Sensors.OverspeedWarning),
			ff(d.Radios.Com1), ff(d.Radios.Com2), ff(d.Radios.Nav1), ff(d.Radios.Nav2), ff(d.Radios.XpdrCode),
			fb(d.Autopilot.Master), ff(d.Autopilot.Heading), ff(d.Autopilot.Altitude), ff(d.Autopilot.VS), ff(d.Autopilot.Speed),
			ff(d.Altimeter),
			fb(d.Lights.Beacon), fb(d.Lights.Strobe), fb(d.Lights.Landing),
			ff(d.Controls.Elevator), ff(d.Controls.Aileron), ff(d.Controls.Rudder),
			ff(d.Controls.Flaps), ff(d.Controls.Spoilers), fb(d.Controls.GearDown),
		})
	}

	// Purge DB after export
	_, err = f.db.Exec(`DELETE FROM flight_data`)
	if err != nil {
		return fmt.Errorf("purge db: %w", err)
	}

	f.mu.Lock()
	f.dataCount = 0
	f.mu.Unlock()

	return nil
}

// startDataStreamLocked starts the continuous data stream goroutine.
// Must be called with f.mu held.
func (f *FlightDataService) startDataStreamLocked() {
	if f.streaming {
		return
	}
	f.streaming = true
	f.streamStopCh = make(chan struct{})
	go f.dataStreamLoop()
}

// stopDataStreamLocked stops the continuous data stream goroutine.
// Must be called with f.mu held.
func (f *FlightDataService) stopDataStreamLocked() {
	if !f.streaming {
		return
	}
	f.streaming = false
	close(f.streamStopCh)
}

// dataStreamLoop is the single goroutine that polls SimConnect.
// It always emits flight-data events, and writes to DB when recording.
// On connection loss it automatically attempts to reconnect with exponential backoff.
func (f *FlightDataService) dataStreamLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var reconnectBackoff time.Duration
	var lastReconnectAttempt time.Time

	for {
		select {
		case <-f.streamStopCh:
			return
		case <-ticker.C:
			f.mu.Lock()
			connector := f.connector
			recording := f.recording
			wasActive := f.simActive
			f.mu.Unlock()

			if connector == nil {
				continue
			}

			data, err := connector.GetFlightData()
			if err != nil {
				if wasActive {
					f.mu.Lock()
					f.simActive = false
					f.mu.Unlock()
					if f.app != nil {
						f.app.Event.Emit("connection-state", "")
					}
					slog.Warn("simulator data lost, will attempt reconnection", "error", err)
					reconnectBackoff = 2 * time.Second
					lastReconnectAttempt = time.Time{}
				}

				// Attempt reconnection with exponential backoff
				if reconnectBackoff > 0 && time.Since(lastReconnectAttempt) >= reconnectBackoff {
					lastReconnectAttempt = time.Now()
					slog.Info("attempting simulator reconnection", "backoff", reconnectBackoff)

					if err := f.attemptReconnect(); err != nil {
						slog.Debug("reconnection attempt failed", "error", err, "next_in", reconnectBackoff*2)
						if reconnectBackoff < 30*time.Second {
							reconnectBackoff *= 2
						}
					} else {
						slog.Info("simulator reconnected", "adapter", connector.Name())
						reconnectBackoff = 0
					}
				}
				continue
			}

			// Data received successfully — reset reconnect state
			reconnectBackoff = 0

			if !wasActive {
				f.mu.Lock()
				f.simActive = true
				f.mu.Unlock()
				if f.app != nil {
					f.app.Event.Emit("connection-state", connector.Name())
				}
				slog.Info("simulator data received", "adapter", connector.Name())
			}

			if f.app != nil {
				f.app.Event.Emit("flight-data", data)
			}

			if recording {
				jsonBytes, err := json.Marshal(data)
				if err != nil {
					slog.Error("failed to marshal flight data", "error", err)
					continue
				}

				_, err = f.db.Exec(
					`INSERT INTO flight_data (data) VALUES (?)`,
					string(jsonBytes),
				)
				if err != nil {
					slog.Error("failed to insert flight data", "error", err)
					continue
				}

				f.mu.Lock()
				f.dataCount++
				f.mu.Unlock()
			}
		}
	}
}

// attemptReconnect disconnects and reconnects the current simulator adapter.
func (f *FlightDataService) attemptReconnect() error {
	f.mu.Lock()
	connector := f.connector
	f.mu.Unlock()

	if connector == nil {
		return fmt.Errorf("no connector")
	}

	connector.Disconnect()
	return connector.Connect()
}

// GetFlightDataNow returns a one-shot read of the current flight data.
func (f *FlightDataService) GetFlightDataNow() (*FlightData, error) {
	f.mu.Lock()
	connector := f.connector
	f.mu.Unlock()

	if connector == nil {
		return nil, fmt.Errorf("no simulator connected")
	}

	return connector.GetFlightData()
}
