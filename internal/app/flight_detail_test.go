package app

import (
	"errors"
	"testing"
)

// flightDetailAPI records the path it was asked for and answers with one
// canned reply.
type flightDetailAPI struct {
	stubAPI
	path string
}

func (*flightDetailAPI) Token() string { return "pilot-token" }
func (s *flightDetailAPI) DoRequest(method, path string, body interface{}) ([]byte, int, error) {
	s.path = path
	return s.stubAPI.DoRequest(method, path, body)
}

func detailApp(status int, body string) (*App, *flightDetailAPI) {
	api := &flightDetailAPI{stubAPI: stubAPI{status: status, body: []byte(body)}}
	return &App{Airspace: api}, api
}

// The reply FerrLab/airspace#1059 sends for a flight with every section
// recorded, trimmed to one entry per list.
const fullFlightDetail = `{"data":{
	"id": 12345, "route": "UZ25 PORTO", "departure_runway": "10L", "arrival_runway": "20L",
	"approach_type": "ils", "landing_g_force": 1.18,
	"landing": {
		"landing_rate_band": "good", "g_force_band": "normal",
		"touchdown_distance_m": 420, "touchdown_offset_m": -1.5,
		"touchdown_ground_speed": 132, "touchdown_indicated_airspeed": 138,
		"runway_used_m": 1150, "runway_used_percent": 87.5,
		"runway": {"designator": "20L", "magnetic_heading": 200, "length_m": 1260, "width_m": 42},
		"wind": {"direction_degrees": 200, "speed_knots": 10, "headwind_knots": 10, "crosswind_knots": 0},
		"stabilized_approach": {"gate_height_ft": 1000, "passed": false,
			"criteria": {"flaps": {"passed": false, "measured_pct": 40, "required_pct": 70},
				"gear": {"passed": true, "down": true},
				"speed": {"passed": true, "measured_kt": 150, "limit_kt": 160, "reference_kt": 135, "margin_kt": 25}},
			"failures": ["Approach unstabilized at 1000 ft above field"]}
	},
	"takeoff": {"liftoff_distance_m": 1900, "takeoff_roll_m": 1750, "liftoff_offset_m": 0.8,
		"liftoff_ground_speed": 151, "liftoff_indicated_airspeed": 155, "runway_used_percent": 51.3,
		"runway": null, "wind": null},
	"score_breakdown": {"score": 8.6, "contributing": ["safety"],
		"categories": [{"key": "safety", "score": 8, "weight": 1, "available": true, "unavailable_reason": null,
			"formula": "10 minus 2 per violation", "inputs": [{"label": "FDM violations", "value": "1"}]}]},
	"events": [{"id": 77, "type": "violation", "event": "Approach unstabilized", "happened_at": "2026-09-26T15:18:00Z", "duration_seconds": null}],
	"points": {"total": -50, "entries": [{"id": 9, "source": "fdm", "reason": "Unstable approach", "points": -50, "remarks": null, "created_at": "2026-09-26T15:31:00Z"}]}
}}`

func TestFlightDetailCarriesTheRecordedAnalysis(t *testing.T) {
	a, api := detailApp(200, fullFlightDetail)

	result, err := a.GetFlightDetail("12345")
	if err != nil {
		t.Fatal(err)
	}
	if api.path != "/api/v2/acars/flights/12345" {
		t.Fatalf("asked the server for %q", api.path)
	}
	if result.Status != "ok" || result.Data == nil {
		t.Fatalf("no detail: %+v", result)
	}
	d := result.Data
	if d.ID != "12345" || d.Route != "UZ25 PORTO" || d.ArrivalRunway != "20L" || d.ApproachType != "ils" {
		t.Fatalf("flight fields lost: %+v", d)
	}
	if d.Landing == nil || *d.Landing.TouchdownDistanceM != 420 || *d.Landing.TouchdownOffsetM != -1.5 || *d.Landing.RunwayUsedM != 1150 {
		t.Fatalf("touchdown analysis lost: %+v", d.Landing)
	}
	if d.Landing.Runway == nil || *d.Landing.Runway.LengthM != 1260 || d.Landing.Wind == nil || d.Landing.Wind.HeadwindKnots != 10 {
		t.Fatalf("runway or wind lost: %+v", d.Landing)
	}
	gate := d.Landing.StabilizedApproach
	if gate == nil || gate.Passed || gate.Criteria == nil || gate.Criteria.Flaps.Passed || *gate.Criteria.Flaps.MeasuredPct != 40 || len(gate.Failures) != 1 {
		t.Fatalf("a failed stabilized gate must reach the pilot as failed: %+v", gate)
	}
	if d.Takeoff == nil || *d.Takeoff.TakeoffRollM != 1750 || d.Takeoff.Runway != nil || d.Takeoff.Wind != nil {
		t.Fatalf("takeoff analysis lost: %+v", d.Takeoff)
	}
	if *d.ScoreBreakdown.Score != 8.6 || len(d.ScoreBreakdown.Categories) != 1 || d.ScoreBreakdown.Categories[0].Inputs[0].Value != "1" {
		t.Fatalf("score breakdown lost: %+v", d.ScoreBreakdown)
	}
	if len(d.Events) != 1 || d.Events[0].ID != "77" || d.Events[0].Type != "violation" || d.Events[0].DurationSeconds != nil {
		t.Fatalf("events lost: %+v", d.Events)
	}
	if d.Points.Total != -50 || len(d.Points.Entries) != 1 || d.Points.Entries[0].ID != "9" || d.Points.Entries[0].Source != "fdm" {
		t.Fatalf("points lost: %+v", d.Points)
	}
}

// A flight with nothing measured must keep its sections nil, so the view
// hides the tabs instead of showing zeros as if they were measurements.
func TestFlightDetailWithNothingRecordedKeepsSectionsEmpty(t *testing.T) {
	a, _ := detailApp(200, `{"data":{"id":7,"route":null,"landing":null,"takeoff":null,"landing_g_force":null,
		"score_breakdown":{"score":null,"contributing":[],"categories":[]},"events":[],"points":{"total":0,"entries":[]}}}`)

	result, err := a.GetFlightDetail("7")
	if err != nil {
		t.Fatal(err)
	}
	d := result.Data
	if d.Landing != nil || d.Takeoff != nil || d.LandingGForce != nil || d.ScoreBreakdown.Score != nil {
		t.Fatalf("an unrecorded section came back filled in: %+v", d)
	}
	if d.Events == nil || d.Points.Entries == nil || d.ScoreBreakdown.Categories == nil {
		t.Fatalf("empty lists must stay lists for the webview: %+v", d)
	}
}

func TestFlightDetailAcceptsTheServerVariants(t *testing.T) {
	cases := map[string]string{
		"flat body":            `{"id":7,"events":[],"points":{"total":0,"entries":[]}}`,
		"string ids":           `{"data":{"id":"7","events":[{"id":"3","type":"warning","event":"Overspeed"}],"points":{"total":5,"entries":[{"id":"1","source":"staff","reason":"Briefing","points":5}]}}}`,
		"lists missing":        `{"data":{"id":7}}`,
		"unknown extra fields": `{"data":{"id":7,"new_section":{"x":1},"events":[]}}`,
	}
	for name, body := range cases {
		a, _ := detailApp(200, body)
		result, err := a.GetFlightDetail("7")
		if err != nil || result.Data == nil || result.Data.ID != "7" {
			t.Fatalf("%s: %+v, %v", name, result, err)
		}
		if result.Data.Events == nil || result.Data.Points.Entries == nil {
			t.Fatalf("%s: nil list reached the webview", name)
		}
	}
}

func TestFlightDetailRejectsGarbageInsteadOfShowingAnEmptyFlight(t *testing.T) {
	for name, body := range map[string]string{
		"not JSON":       `error code: 502`,
		"no id":          `{"data":{"route":"DCT"}}`,
		"id is an array": `{"data":{"id":[1]}}`,
		"wrong type":     `{"data":{"id":7,"landing":{"touchdown_distance_m":"far"}}}`,
		"bad event id":   `{"data":{"id":7,"events":[{"id":{"x":1}}]}}`,
	} {
		a, _ := detailApp(200, body)
		result, err := a.GetFlightDetail("7")
		if !errors.Is(err, errInvalidFlightDetail) {
			t.Fatalf("%s: accepted as %+v (err %v)", name, result, err)
		}
	}
}

func TestFlightDetailReportsWhyItIsUnavailable(t *testing.T) {
	for status, want := range map[int]string{403: "accessDenied", 404: "unavailable", 429: "rateLimited"} {
		a, _ := detailApp(status, `not JSON`)
		result, err := a.GetFlightDetail("7")
		if err != nil || result.Status != want || result.Data != nil {
			t.Fatalf("status %d: %+v, %v", status, result, err)
		}
	}

	local := &App{Airspace: &flightDetailAPI{}}
	local.settings.LocalMode = true
	if result, err := local.GetFlightDetail("7"); err != nil || result.Status != "localMode" {
		t.Fatalf("local mode: %+v, %v", result, err)
	}
}

func TestFlightDetailSurfacesServerErrors(t *testing.T) {
	a, _ := detailApp(500, `oops`)
	if _, err := a.GetFlightDetail("7"); err == nil {
		t.Fatal("a 500 was shown as a flight")
	}
}

func TestFlightDetailRefusesIDsThatAreNotOnePathSegment(t *testing.T) {
	for _, id := range []string{"", ".", "..", "7/track", "../pilot", "7?x=1", "7#a", "%2e%2e", `7\x`} {
		a, api := detailApp(200, fullFlightDetail)
		if _, err := a.GetFlightDetail(id); !errors.Is(err, errInvalidFlightID) {
			t.Fatalf("id %q was sent to the server", id)
		}
		if _, err := a.GetFlightTrack(id); !errors.Is(err, errInvalidFlightID) {
			t.Fatalf("track id %q was sent to the server", id)
		}
		if api.calls != 0 {
			t.Fatalf("id %q reached the network", id)
		}
	}
}

func TestFlightTrackCarriesTheRecordedPositionsInOrder(t *testing.T) {
	a, api := detailApp(200, `{"data":[
		{"sent_at":"2026-09-26T14:40:00.250Z","phase":"Climb","lat":-23.4,"lng":-46.4,"altitude":2500,"altitude_agl":null,"indicated_airspeed":200,"ground_speed":210,"vertical_speed":2000,"heading":80},
		{"sent_at":"2026-09-26T14:50:00.000Z","phase":"Cruise","lat":40.1,"lng":180.5,"altitude":30000,"altitude_agl":29000,"indicated_airspeed":280,"ground_speed":450,"vertical_speed":0,"heading":null}
	]}`)

	result, err := a.GetFlightTrack("7")
	if err != nil {
		t.Fatal(err)
	}
	if api.path != "/api/v2/acars/flights/7/track" {
		t.Fatalf("asked the server for %q", api.path)
	}
	if result.Status != "ok" || len(result.Points) != 2 {
		t.Fatalf("track holds %d of 2 samples: %+v", len(result.Points), result)
	}
	first, second := result.Points[0], result.Points[1]
	if first.Phase != "Climb" || first.AltitudeAGL != nil || *first.Heading != 80 {
		t.Fatalf("first sample: %+v", first)
	}
	if second.Lng != 180.5 || *second.AltitudeAGL != 29000 || second.Heading != nil {
		t.Fatalf("an unwrapped longitude or a missing heading was altered: %+v", second)
	}
}

func TestFlightTrackWithoutPositionsIsAnEmptyTrack(t *testing.T) {
	a, _ := detailApp(200, `{"data":[]}`)
	result, err := a.GetFlightTrack("7")
	if err != nil || result.Status != "ok" || result.Points == nil || len(result.Points) != 0 {
		t.Fatalf("%+v, %v", result, err)
	}
}

func TestFlightTrackRejectsGarbage(t *testing.T) {
	for name, body := range map[string]string{
		"not JSON":    `<html>`,
		"no data":     `{"points":[]}`,
		"data object": `{"data":{"lat":1}}`,
		"wrong type":  `{"data":[{"lat":"north"}]}`,
	} {
		a, _ := detailApp(200, body)
		if _, err := a.GetFlightTrack("7"); !errors.Is(err, errInvalidFlightTrack) {
			t.Fatalf("%s: accepted (err %v)", name, err)
		}
	}
}

func TestFlightTrackReportsWhyItIsUnavailable(t *testing.T) {
	a, _ := detailApp(404, `not JSON`)
	result, err := a.GetFlightTrack("7")
	if err != nil || result.Status != "unavailable" || len(result.Points) != 0 {
		t.Fatalf("%+v, %v", result, err)
	}
}
