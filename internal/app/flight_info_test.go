package app

import (
	"strconv"
	"testing"
	"time"
)

// When flight is idle, GetActiveFlightInfo returns an empty map so no stale
// tracking data is exposed.
func TestGetActiveFlightInfoIdle(t *testing.T) {
	app := &App{
		state: "idle",
	}

	info := app.GetActiveFlightInfo()
	if len(info) != 0 {
		t.Fatalf("GetActiveFlightInfo() = %v, want empty map when idle", info)
	}
}

// When flight is active, GetActiveFlightInfo returns callsign, route, finish cooldown,
// startTimeMs, and resolved airport coordinates for departure and arrival.
func TestGetActiveFlightInfoActive(t *testing.T) {
	startTime := time.Now().Add(-5 * time.Minute)
	app := &App{
		state:     "active",
		callsign:  "AZU123",
		departure: "SBGR",
		arrival:   "SBRJ",
		startTime: startTime,
	}

	info := app.GetActiveFlightInfo()

	if info["callsign"] != "AZU123" {
		t.Errorf("callsign = %q, want %q", info["callsign"], "AZU123")
	}
	if info["departure"] != "SBGR" {
		t.Errorf("departure = %q, want %q", info["departure"], "SBGR")
	}
	if info["arrival"] != "SBRJ" {
		t.Errorf("arrival = %q, want %q", info["arrival"], "SBRJ")
	}

	// startTimeMs must be present and parse accurately to startTime epoch ms
	rawStartMs, ok := info["startTimeMs"]
	if !ok {
		t.Fatal("startTimeMs key missing from GetActiveFlightInfo() output")
	}
	startMs, err := strconv.ParseInt(rawStartMs, 10, 64)
	if err != nil {
		t.Fatalf("startTimeMs %q is not a valid int64: %v", rawStartMs, err)
	}
	if diff := startMs - startTime.UnixMilli(); diff < -10 || diff > 10 {
		t.Errorf("startTimeMs = %d, want ~%d (diff %d ms)", startMs, startTime.UnixMilli(), diff)
	}

	// Departure coordinates for SBGR
	if info["departureLat"] == "" || info["departureLon"] == "" {
		t.Errorf("departureLat / departureLon missing for SBGR: lat=%q, lon=%q", info["departureLat"], info["departureLon"])
	}

	// Arrival coordinates for SBRJ
	if info["arrivalLat"] == "" || info["arrivalLon"] == "" {
		t.Errorf("arrivalLat / arrivalLon missing for SBRJ: lat=%q, lon=%q", info["arrivalLat"], info["arrivalLon"])
	}
}

// GetAirportCoordinates returns resolved coordinates for known ICAO codes
// and nil for unknown airport codes.
func TestGetAirportCoordinates(t *testing.T) {
	app := &App{}

	coords := app.GetAirportCoordinates("SBGR")
	if coords == nil {
		t.Fatal("GetAirportCoordinates(\"SBGR\") = nil, want valid coordinates")
	}
	if coords.Latitude == 0 || coords.Longitude == 0 {
		t.Errorf("GetAirportCoordinates(\"SBGR\") = %+v, want non-zero coordinates", coords)
	}

	unknown := app.GetAirportCoordinates("ZZZZ")
	if unknown != nil {
		t.Errorf("GetAirportCoordinates(\"ZZZZ\") = %+v, want nil", unknown)
	}
}
