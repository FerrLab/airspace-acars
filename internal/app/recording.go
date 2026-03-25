package app

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"airspace-acars/internal/domain"
)

// StartRecording begins recording flight data to the database.
func (a *App) StartRecording() error {
	a.simMu.Lock()
	defer a.simMu.Unlock()

	if a.connector == nil {
		return fmt.Errorf("no simulator connected")
	}
	if a.recording {
		return fmt.Errorf("already recording")
	}

	a.recording = true
	a.recStartTime = time.Now()
	a.dataCount = 0
	a.UI.EmitEvent("recording-state", true)
	return nil
}

// StopRecording stops recording flight data.
func (a *App) StopRecording() {
	a.simMu.Lock()
	defer a.simMu.Unlock()

	if !a.recording {
		return
	}
	a.recording = false
	a.UI.EmitEvent("recording-state", false)
}

// IsRecording returns whether flight data is being recorded.
func (a *App) IsRecording() bool {
	a.simMu.Lock()
	defer a.simMu.Unlock()
	return a.recording
}

// GetRecordingInfo returns the current recording state.
func (a *App) GetRecordingInfo() map[string]interface{} {
	a.simMu.Lock()
	defer a.simMu.Unlock()

	duration := 0.0
	if a.recording {
		duration = time.Since(a.recStartTime).Seconds()
	}

	return map[string]interface{}{
		"recording": a.recording,
		"duration":  duration,
		"dataCount": a.dataCount,
	}
}

// ExportCSV exports recorded data to a CSV file and purges the database.
func (a *App) ExportCSV(filePath string) error {
	rows, err := a.DB.QueryFlightData()
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

		var d domain.FlightData
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

	if err := a.DB.PurgeFlightData(); err != nil {
		return fmt.Errorf("purge db: %w", err)
	}

	a.simMu.Lock()
	a.dataCount = 0
	a.simMu.Unlock()

	return nil
}
