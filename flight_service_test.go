package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeasurement(t *testing.T) {
	got := m(123.4, "ft")
	assert.Equal(t, 123.4, got.Value)
	assert.Equal(t, "ft", got.Unit)
}

func TestPositionIntervalOrdering(t *testing.T) {
	// Intervals should be ordered: critical < low < high < static
	assert.Less(t, posIntervalCritical, posIntervalLow)
	assert.Less(t, posIntervalLow, posIntervalHigh)
	assert.Less(t, posIntervalHigh, posIntervalStatic)
}

func TestBuildPositionReport(t *testing.T) {
	mock := &MockSimConnector{data: sampleFlightData(), name: "TestSim"}
	fds := &FlightDataService{connector: mock, simActive: true}

	f := &FlightService{
		auth:       &AuthService{},
		flightData: fds,
		state:      "active",
		callsign:   "BAW123",
		departure:  "EGLL",
		arrival:    "KJFK",
		startTime:  time.Now().Add(-10 * time.Minute),
	}

	report := f.buildPositionReport(sampleFlightData())

	// Top-level fields
	assert.Equal(t, "BAW123", report["callsign"])
	assert.Equal(t, "EGLL", report["departure"])
	assert.Equal(t, "KJFK", report["arrival"])
	assert.Equal(t, "TestSim", report["simulator"])
	assert.Contains(t, report, "timestamp")
	assert.Contains(t, report, "acarsVersion")

	// Elapsed time
	elapsed := report["elapsedTime"].(measurement)
	assert.Equal(t, "s", elapsed.Unit)
	elapsedVal, ok := elapsed.Value.(float64)
	require.True(t, ok)
	assert.Greater(t, elapsedVal, 0.0)

	// Position
	pos := report["position"].(map[string]interface{})
	lat := pos["latitude"].(measurement)
	assert.Equal(t, 51.4775, lat.Value)
	assert.Equal(t, "deg", lat.Unit)

	// Attitude
	att := report["attitude"].(map[string]interface{})
	gs := att["gs"].(measurement)
	assert.Equal(t, 0.0, gs.Value)
	assert.Equal(t, "kts", gs.Unit)

	// Engines
	engines := report["engines"].([]map[string]interface{})
	assert.Len(t, engines, 4)
	assert.Equal(t, true, engines[0]["exists"])
	assert.Equal(t, true, engines[0]["running"])
	n1 := engines[0]["n1"].(measurement)
	assert.Equal(t, 22.5, n1.Value)

	// Sensors
	sensors := report["sensors"].(map[string]interface{})
	assert.Equal(t, true, sensors["onGround"])

	// Radios
	radios := report["radios"].(map[string]interface{})
	com1 := radios["com1"].(measurement)
	assert.Equal(t, 118.3, com1.Value)
	assert.Equal(t, "stand-by", radios["transponderState"])

	// Autopilot
	ap := report["autopilot"].(map[string]interface{})
	assert.Equal(t, false, ap["master"])

	// Altimeter
	alt := report["altimeter"].(measurement)
	assert.Equal(t, 29.92, alt.Value)
	assert.Equal(t, "inHg", alt.Unit)

	// Lights
	lights := report["lights"].(map[string]interface{})
	assert.Equal(t, false, lights["beacon"])

	// Controls
	controls := report["controls"].(map[string]interface{})
	assert.Equal(t, false, controls["gearDown"])

	// APU
	apu := report["apu"].(map[string]interface{})
	assert.Equal(t, false, apu["switchOn"])

	// Doors
	doors := report["doors"].([]map[string]interface{})
	assert.Len(t, doors, 5)

	// SimTime
	st := report["simTime"].(map[string]interface{})
	zuluHour := st["zuluHour"].(measurement)
	assert.Equal(t, 12, zuluHour.Value)

	// Weight
	weight := report["weight"].(map[string]interface{})
	totalWeight := weight["total"].(measurement)
	assert.Equal(t, 130000.0, totalWeight.Value)
	assert.Equal(t, "lbs", totalWeight.Unit)

	// Aircraft name
	assert.Equal(t, "Boeing 737-800", report["aircraftName"])
}
