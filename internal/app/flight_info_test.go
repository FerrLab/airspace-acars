package app

import (
	"strconv"
	"testing"
	"time"
)

// With no flight under way there is nothing to report, so a reloaded
// dashboard shows no stale flight.
func TestAnIdleAppReportsNoActiveFlight(t *testing.T) {
	a := &App{state: "idle"}

	if info := a.GetActiveFlightInfo(); len(info) != 0 {
		t.Fatalf("GetActiveFlightInfo() = %v while idle; the dashboard would show a flight that is not running", info)
	}
}

// The dashboard's elapsed time is counted from startTimeMs, so it survives a
// reload of the webview mid-flight instead of starting again from zero.
func TestTheActiveFlightReportsWhenItStarted(t *testing.T) {
	started := time.Now().Add(-5 * time.Minute)
	a := &App{state: "active", callsign: "AZU123", departure: "SBGR", arrival: "SBRJ", startTime: started}

	info := a.GetActiveFlightInfo()

	if info["callsign"] != "AZU123" || info["departure"] != "SBGR" || info["arrival"] != "SBRJ" {
		t.Errorf("flight = %s %s-%s, want AZU123 SBGR-SBRJ", info["callsign"], info["departure"], info["arrival"])
	}
	startMs, err := strconv.ParseInt(info["startTimeMs"], 10, 64)
	if err != nil {
		t.Fatalf("startTimeMs %q is not a Unix millisecond time: %v", info["startTimeMs"], err)
	}
	if startMs != started.UnixMilli() {
		t.Errorf("startTimeMs = %d, want %d; the elapsed time would be off after a reload", startMs, started.UnixMilli())
	}
}
