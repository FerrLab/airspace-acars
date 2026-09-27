package app

import (
	"net/url"
	"testing"
)

type flightLogAPI struct {
	stubAPI
	path      string
	pilotBody []byte
}

func (*flightLogAPI) Token() string { return "pilot-token" }
func (s *flightLogAPI) DoRequest(method, path string, body interface{}) ([]byte, int, error) {
	s.path = path
	if path == "/api/v2/acars/pilot" {
		if s.pilotBody != nil {
			return s.pilotBody, 200, nil
		}
		return []byte(`{"id": 42, "name": "Pilot 42", "callsign": "GLO1234"}`), 200, nil
	}
	return s.stubAPI.DoRequest(method, path, body)
}

func TestGetMyFlightsSuccessWithNestedDataAndStats(t *testing.T) {
	payload := `{
		"data": [
			{
				"id": 101,
				"callsign": "GLO1234",
				"flight_number": "1234",
				"status": "accepted",
				"departure_airport": {
					"icao": "SBGR",
					"name": "Guarulhos",
					"city": "Sao Paulo"
				},
				"arrival_airport": {
					"icao": "SBRJ",
					"name": "Santos Dumont",
					"city": "Rio de Janeiro"
				},
				"aircraft": {
					"registration": "PR-GXZ",
					"name": "Boeing 737-800",
					"icao_code": "B738"
				},
				"flight_time": 55,
				"distance": 196,
				"landing_rate": -142.5,
				"fuel_used": 1450.0,
				"score": 98,
				"created_at": "2026-09-20T14:30:00Z"
			},
			{
				"id": 102,
				"callsign": "GLO1235",
				"flight_number": "1235",
				"status": "accepted",
				"departure_airport_icao": "SBRJ",
				"arrival_airport_icao": "SBBR",
				"aircraft": {
					"registration": "PR-GXZ",
					"icao": "B738"
				},
				"flight_time": 95,
				"distance": 0,
				"landing_rate": -180.0,
				"fuel_used": 2100.0,
				"score": 95,
				"created_at": "2026-09-21T18:00:00Z"
			}
		],
		"meta": {
			"current_page": 1,
			"last_page": 3,
			"total": 12
		}
	}`

	api := &flightLogAPI{stubAPI: stubAPI{
		status: 200,
		body:   []byte(payload),
	}}

	a := &App{Airspace: api}

	res, err := a.GetMyFlights(1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "ok" {
		t.Fatalf("got status %q, want 'ok'", res.Status)
	}

	if len(res.Flights) != 2 {
		t.Fatalf("got %d flights, want 2", len(res.Flights))
	}

	// Flight 1: coordinate resolution from dictionary
	f1 := res.Flights[0]
	if f1.DepartureAirport.ICAO != "SBGR" || f1.DepartureAirport.Latitude == 0 {
		t.Errorf("expected SBGR coords to be resolved, got lat=%v", f1.DepartureAirport.Latitude)
	}
	if f1.ArrivalAirport.ICAO != "SBRJ" || f1.ArrivalAirport.Longitude == 0 {
		t.Errorf("expected SBRJ coords to be resolved, got lon=%v", f1.ArrivalAirport.Longitude)
	}
	if f1.Aircraft == nil || f1.Aircraft.ICAOCode != "B738" {
		t.Errorf("expected aircraft B738, got %+v", f1.Aircraft)
	}
	if f1.LandingRateFPM != -142.5 {
		t.Errorf("expected landing rate -142.5, got %v", f1.LandingRateFPM)
	}

	// Flight 2: flat ICAO with 0 distance triggering Great Circle calculation
	f2 := res.Flights[1]
	if f2.DepartureAirport.ICAO != "SBRJ" || f2.ArrivalAirport.ICAO != "SBBR" {
		t.Errorf("expected SBRJ->SBBR, got %s->%s", f2.DepartureAirport.ICAO, f2.ArrivalAirport.ICAO)
	}
	if f2.DistanceNM <= 0 {
		t.Errorf("expected computed distance > 0, got %v", f2.DistanceNM)
	}

	// Summary stats: verifies meta.total (12) is preferred over page count (2)
	if res.Pilot.TotalFlights != 12 {
		t.Errorf("expected TotalFlights 12, got %d", res.Pilot.TotalFlights)
	}
	if res.Pilot.AvgLandingRate == 0 {
		t.Errorf("expected non-zero AvgLandingRate")
	}

	u, err := url.Parse(api.path)
	if err != nil {
		t.Fatalf("failed to parse path: %v", err)
	}
	if u.Query().Get("page") != "1" || u.Query().Get("per_page") != "10" {
		t.Errorf("unexpected query parameters in %s", api.path)
	}
	if u.Query().Get("filter[user_id]") != "42" {
		t.Errorf("expected filter[user_id]=42, got %s", u.Query().Get("filter[user_id]"))
	}
}

func TestGetMyFlightsLocalModeAndNoSession(t *testing.T) {
	api := &flightLogAPI{}
	a := &App{Airspace: api}
	a.settings.LocalMode = true
	res, err := a.GetMyFlights(1, 20)
	if err != nil || res.Status != "localMode" || api.calls != 0 {
		t.Fatalf("unexpected localMode response: %+v, err: %v", res, err)
	}

	noSessionAPI := &stubAPI{}
	a = &App{Airspace: noSessionAPI}
	res, err = a.GetMyFlights(1, 20)
	if err != nil || res.Status != "noSession" || noSessionAPI.calls != 0 {
		t.Fatalf("unexpected noSession response: %+v, err: %v", res, err)
	}
}

func TestGetMyFlightsErrorStatuses(t *testing.T) {
	for status, want := range map[int]string{
		401: "accessDenied",
		403: "accessDenied",
		404: "unavailable",
		429: "rateLimited",
	} {
		api := &flightLogAPI{stubAPI: stubAPI{status: status, body: []byte("error")}}
		a := &App{Airspace: api}
		res, err := a.GetMyFlights(1, 20)
		if err != nil || res.Status != want {
			t.Errorf("status %d: got status %q, want %q", status, res.Status, want)
		}
	}
}

func TestGetMyFlightsFailsWithoutPilotID(t *testing.T) {
	// If /api/v2/acars/pilot returns no id, user_id or pilot_id, GetMyFlights must fail
	// and never issue an unfiltered request.
	api := &flightLogAPI{
		stubAPI:   stubAPI{status: 200, body: []byte(`{"data": []}`)},
		pilotBody: []byte(`{"name": "No ID Pilot"}`),
	}
	a := &App{Airspace: api}
	res, err := a.GetMyFlights(1, 20)
	if err == nil {
		t.Fatal("expected error when pilot ID cannot be determined, got nil")
	}
	if res.Status != "unavailable" {
		t.Errorf("got status %q, want 'unavailable'", res.Status)
	}
}

func TestNullIslandDistanceGuard(t *testing.T) {
	// If only one airport resolves coordinates, distance must not be measured to (0,0)
	d := calculateGreatCircleDistanceNM(-23.435556, -46.473056, 0, 0)
	if d != 0 {
		t.Errorf("expected 0 distance when destination is (0,0), got %v", d)
	}
	d2 := calculateGreatCircleDistanceNM(0, 0, -22.910278, -43.163056)
	if d2 != 0 {
		t.Errorf("expected 0 distance when origin is (0,0), got %v", d2)
	}
}
