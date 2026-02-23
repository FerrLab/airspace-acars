package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type FlightDataService struct {
	db        *sql.DB
	app       *application.App
	connector SimConnector
	mu        sync.Mutex
	recording bool
	stopCh    chan struct{}
	startTime time.Time
	dataCount int
}

func NewFlightDataService(db *sql.DB) *FlightDataService {
	return &FlightDataService{
		db: db,
	}
}

func (f *FlightDataService) setApp(app *application.App) {
	f.app = app
}

func (f *FlightDataService) ConnectSim(simType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.connector != nil {
		f.connector.Disconnect()
	}

	var connector SimConnector
	var err error

	switch simType {
	case "xplane":
		connector = NewXPlaneAdapter("127.0.0.1", 49000)
	case "simconnect":
		connector = NewSimConnectAdapter()
		if connector == nil {
			return fmt.Errorf("SimConnect not available on this platform")
		}
	default: // "auto"
		connector = NewSimConnectAdapter()
		if connector == nil {
			connector = NewXPlaneAdapter("127.0.0.1", 49000)
		}
	}

	if err = connector.Connect(); err != nil {
		return fmt.Errorf("connect to %s: %w", connector.Name(), err)
	}

	f.connector = connector
	slog.Info("connected to simulator", "adapter", connector.Name())
	return nil
}

func (f *FlightDataService) DisconnectSim() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.connector != nil {
		f.connector.Disconnect()
		f.connector = nil
	}
}

func (f *FlightDataService) IsConnected() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connector != nil
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
	f.stopCh = make(chan struct{})
	f.startTime = time.Now()
	f.dataCount = 0

	go f.recordLoop()

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
	close(f.stopCh)

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
	rows, err := f.db.Query(`SELECT timestamp, altitude, heading, pitch, roll, airspeed, ground_speed, vertical_speed FROM flight_data ORDER BY id`)
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

	w.Write([]string{"timestamp", "altitude", "heading", "pitch", "roll", "airspeed", "ground_speed", "vertical_speed"})

	for rows.Next() {
		var ts string
		var alt, hdg, pitch, roll, aspd, gspd, vspd float64
		if err := rows.Scan(&ts, &alt, &hdg, &pitch, &roll, &aspd, &gspd, &vspd); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}
		w.Write([]string{
			ts,
			strconv.FormatFloat(alt, 'f', 2, 64),
			strconv.FormatFloat(hdg, 'f', 2, 64),
			strconv.FormatFloat(pitch, 'f', 2, 64),
			strconv.FormatFloat(roll, 'f', 2, 64),
			strconv.FormatFloat(aspd, 'f', 2, 64),
			strconv.FormatFloat(gspd, 'f', 2, 64),
			strconv.FormatFloat(vspd, 'f', 2, 64),
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

func (f *FlightDataService) recordLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-f.stopCh:
			return
		case <-ticker.C:
			f.mu.Lock()
			connector := f.connector
			f.mu.Unlock()

			if connector == nil {
				continue
			}

			data, err := connector.GetFlightData()
			if err != nil {
				slog.Error("failed to get flight data", "error", err)
				continue
			}

			_, err = f.db.Exec(
				`INSERT INTO flight_data (altitude, heading, pitch, roll, airspeed, ground_speed, vertical_speed) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				data.Altitude, data.Heading, data.Pitch, data.Roll, data.Airspeed, data.GroundSpeed, data.VerticalSpeed,
			)
			if err != nil {
				slog.Error("failed to insert flight data", "error", err)
				continue
			}

			f.mu.Lock()
			f.dataCount++
			f.mu.Unlock()

			if f.app != nil {
				f.app.Event.Emit("flight-data", data)
			}
		}
	}
}
