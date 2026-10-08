package app

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"airspace-acars/internal/domain"
)

// The debug screen shows the report that went out, not a rebuilt copy.
func TestASentReportIsKeptAsSent(t *testing.T) {
	a := &App{Airspace: &fixedAPI{status: 202}, DB: newMemDB(), UI: nullUI{}, reports: &reportTap{}}
	u := a.startPositionUploader("booking-718")
	u.Submit(samples(3))
	u.Stop()

	s := a.reports.snapshot()
	if s.Outcome != domain.ReportSent || s.BatchSize != 3 {
		t.Fatalf("kept %q from a batch of %d, want sent from 3", s.Outcome, s.BatchSize)
	}
	if !strings.Contains(s.JSON, `"timestamp": 2`) {
		t.Errorf("kept %s, want the batch's last report", s.JSON)
	}
}

// A report that failed is shown as waiting in the outbox; one with no
// booking to be filed under is shown as dropped.
func TestAFailedReportIsKeptWithItsFate(t *testing.T) {
	quickRetries(t)
	for _, tc := range []struct {
		booking, want string
	}{
		{"booking-718", domain.ReportOutbox},
		{"", domain.ReportDropped},
	} {
		a := &App{Airspace: &fixedAPI{status: 502}, DB: newMemDB(), UI: nullUI{}, reports: &reportTap{}}
		u := a.startPositionUploader(tc.booking)
		u.Submit(samples(2))
		u.Stop()
		if s := a.reports.snapshot(); s.Outcome != tc.want {
			t.Errorf("booking %q: outcome %q, want %q", tc.booking, s.Outcome, tc.want)
		}
	}
}

// Before a flight the screen shows the report that would be sent, built by
// the same code, with runway and stand, and no elapsed time.
func TestOutsideAFlightThePreviewIsTheReportThatWouldBeSent(t *testing.T) {
	a := &App{reports: &reportTap{}}
	l := newTestLocator(&fakeLayouts{layout: egll()})
	a.observeGround(l.groundLocator, onRunway09L())
	l.serve()
	a.observeGround(l.groundLocator, onRunway09L())

	s := a.reports.snapshot()
	if s.Outcome != domain.ReportPreview {
		t.Fatalf("outcome %q, want preview", s.Outcome)
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(s.JSON), &got); err != nil {
		t.Fatalf("preview is not JSON: %v\n%s", err, s.JSON)
	}
	if !strings.Contains(s.JSON, `"designator": "09L"`) {
		t.Errorf("preview lacks the runway the aircraft is on:\n%s", s.JSON)
	}
	if v := got["elapsedTime"].(map[string]interface{})["value"]; v != float64(0) {
		t.Errorf("elapsed time before a flight = %v, want 0", v)
	}
}

// During a flight the uploader's report stays on screen: the 1 Hz preview
// must not replace it.
func TestDuringAFlightAPreviewDoesNotReplaceTheSentReport(t *testing.T) {
	a := &App{Airspace: &fixedAPI{status: 202}, DB: newMemDB(), UI: nullUI{}, reports: &reportTap{}, state: "active"}
	u := a.startPositionUploader("booking-718")
	u.Submit(samples(1))
	u.Stop()

	a.observeGround(newTestLocator(&fakeLayouts{}).groundLocator, onRunway09L())
	if s := a.reports.snapshot(); s.Outcome != domain.ReportSent {
		t.Fatalf("outcome %q after a preview tick mid-flight, want sent", s.Outcome)
	}
}

// A reading that is not a number shows as null, as it was sent.
func TestASnapshotEncodesABadReadingAsNull(t *testing.T) {
	tap := &reportTap{}
	tap.record([]map[string]interface{}{{"altitudeAgl": m(math.NaN(), "ft")}}, domain.ReportSent)
	if s := tap.snapshot(); !strings.Contains(s.JSON, `"value": null`) {
		t.Fatalf("NaN reading shown as %s, want null", s.JSON)
	}
	var none *reportTap
	if s := none.snapshot(); s.JSON != "" {
		t.Fatalf("a nil tap returned %+v", s)
	}
}

// One call gives the screen both halves; with nothing connected it is empty,
// not a crash.
func TestTheDebugSnapshotCombinesGroundAndReport(t *testing.T) {
	a := &App{reports: &reportTap{}}
	l := newTestLocator(&fakeLayouts{layout: egll()})
	a.locator = l.groundLocator
	a.observeGround(a.locator, onRunway09L())
	l.serve()
	a.observeGround(a.locator, onRunway09L())

	snap := a.DebugSnapshot()
	if snap.Ground.Runway != "09L/27R" || snap.Report.Outcome != domain.ReportPreview {
		t.Fatalf("snapshot = %+v", snap)
	}
	if empty := (&App{}).DebugSnapshot(); empty.Ground.Airport != "" || empty.Report.JSON != "" {
		t.Fatalf("snapshot with nothing connected = %+v", empty)
	}
}

// The generated bindings type profiles as an array, so it goes out as [],
// never null, when no aircraft profile is active.
func TestTheDebugSnapshotSendsProfilesAsAnArray(t *testing.T) {
	raw, err := json.Marshal((&App{}).DebugSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"profiles":[]`) {
		t.Fatalf("snapshot encodes profiles as %s, want []", raw)
	}
}
