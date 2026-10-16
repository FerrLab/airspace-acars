package app

import (
	"encoding/json"
	"net/url"
	"testing"
)

type flightLogAPI struct {
	stubAPI
	path        string
	pilotBody   []byte
	pilotStatus int
}

func (*flightLogAPI) Token() string { return "pilot-token" }
func (s *flightLogAPI) DoRequest(method, path string, body interface{}) ([]byte, int, error) {
	s.path = path
	if path == "/api/v2/acars/pilot" {
		if s.pilotStatus != 0 {
			return s.pilotBody, s.pilotStatus, nil
		}
		if s.pilotBody != nil {
			return s.pilotBody, 200, nil
		}
		return []byte(`{"id": 42, "name": "Pilot 42", "callsign": "GLO1234", "landing_rate_avg": -161.25, "flight_hours": 2.5, "total_distance": 196.0}`), 200, nil
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

func TestGetMyFlightsPilotProfileErrors(t *testing.T) {
	for status, want := range map[int]string{
		401: "accessDenied",
		403: "accessDenied",
		404: "unavailable",
		429: "rateLimited",
	} {
		api := &flightLogAPI{pilotStatus: status, pilotBody: []byte("error")}
		a := &App{Airspace: api}
		res, err := a.GetMyFlights(1, 20)
		if err != nil {
			t.Fatalf("pilot status %d: unexpected error %v", status, err)
		}
		if res.Status != want {
			t.Errorf("pilot status %d: got status %q, want %q", status, res.Status, want)
		}
	}

	// 500 or network error should return err so the UI displays retryable banner
	serverErrAPI := &flightLogAPI{pilotStatus: 500, pilotBody: []byte("internal server error")}
	a := &App{Airspace: serverErrAPI}
	_, err := a.GetMyFlights(1, 20)
	if err == nil {
		t.Fatal("expected error on 500 pilot profile response, got nil")
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

func TestPageSumDoesNotMasqueradeAsGlobalStats(t *testing.T) {
	// When pilot profile lacks aggregates and total flights (100) > page flights (1),
	// fallback must not claim the 1-flight distance/hours is the pilot's global total.
	payloadIncomplete := `{
		"data": [{
			"id": 1,
			"callsign": "GLO1",
			"flight_time": 60,
			"distance": 200,
			"landing_rate": -150
		}],
		"meta": {"total": 100, "current_page": 1, "last_page": 5}
	}`
	apiIncomplete := &flightLogAPI{
		stubAPI:   stubAPI{status: 200, body: []byte(payloadIncomplete)},
		pilotBody: []byte(`{"id": 42, "name": "Pilot 42", "callsign": "GLO42"}`),
	}
	a := &App{Airspace: apiIncomplete}
	res, err := a.GetMyFlights(1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pilot.HasGlobalDistance {
		t.Error("expected HasGlobalDistance to be false")
	}
	if res.Pilot.TotalDistanceNM != 0 {
		t.Errorf("expected TotalDistanceNM 0 for incomplete history, got %v", res.Pilot.TotalDistanceNM)
	}

	// Conversely, when total flights (1) == page flights (1), the page IS the full history.
	payloadComplete := `{
		"data": [{
			"id": 1,
			"callsign": "GLO1",
			"flight_time": 60,
			"distance": 200,
			"landing_rate": -150
		}],
		"meta": {"total": 1, "current_page": 1, "last_page": 1}
	}`
	apiComplete := &flightLogAPI{
		stubAPI:   stubAPI{status: 200, body: []byte(payloadComplete)},
		pilotBody: []byte(`{"id": 42, "name": "Pilot 42", "callsign": "GLO42"}`),
	}
	a = &App{Airspace: apiComplete}
	resComplete, err := a.GetMyFlights(1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if resComplete.Pilot.TotalDistanceNM != 200 {
		t.Errorf("expected TotalDistanceNM 200 for complete history, got %v", resComplete.Pilot.TotalDistanceNM)
	}
}

func TestFlightWireRejectsNonFiniteNumbers(t *testing.T) {
	var w flightWire
	if err := json.Unmarshal([]byte(`{"id":1,"distance":"NaN","landing_rate":"1e999","flight_time":"Inf","score":"-1e999","fuel_used":"abc"}`), &w); err != nil {
		t.Fatal(err)
	}
	fl, err := w.toDomain()
	if err != nil {
		t.Fatal(err)
	}
	if fl.DistanceNM != 0 || fl.LandingRateFPM != 0 || fl.FlightTimeMinutes != 0 || fl.Score != 0 || fl.FuelUsedKG != 0 {
		t.Fatalf("non-finite inputs must become 0, got %+v", fl)
	}
	if _, err := json.Marshal(fl); err != nil {
		t.Fatalf("flight must stay serialisable: %v", err)
	}
}

func TestFlightWireRouteAndTrackCoordinatesVariants(t *testing.T) {
	// Variant 1: standard string route and [][2]float64 coordinates
	payloadPairs := `{"id":1,"route":"BITIS UZ25 KASUK","track_coordinates":[[-23.4356,-46.4731],[-25.5958,-54.4875]]}`
	var w1 flightWire
	if err := json.Unmarshal([]byte(payloadPairs), &w1); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	fl1, err := w1.toDomain()
	if err != nil {
		t.Fatalf("toDomain error: %v", err)
	}
	if fl1.Route != "BITIS UZ25 KASUK" {
		t.Errorf("expected route %q, got %q", "BITIS UZ25 KASUK", fl1.Route)
	}
	if len(fl1.TrackCoordinates) != 2 {
		t.Fatalf("expected 2 coordinates, got %d", len(fl1.TrackCoordinates))
	}
	if fl1.TrackCoordinates[0][0] != -23.4356 || fl1.TrackCoordinates[0][1] != -46.4731 {
		t.Errorf("unexpected coord[0]: %v", fl1.TrackCoordinates[0])
	}

	// Variant 2: track_coordinates as objects with lat/lon and latitude/longitude
	payloadObjects := `{"id":2,"track_coordinates":[{"lat":-23.4356,"lon":-46.4731},{"latitude":-25.5958,"longitude":-54.4875}]}`
	var w2 flightWire
	if err := json.Unmarshal([]byte(payloadObjects), &w2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	fl2, err := w2.toDomain()
	if err != nil {
		t.Fatalf("toDomain error: %v", err)
	}
	if len(fl2.TrackCoordinates) != 2 {
		t.Fatalf("expected 2 coordinates from object shape, got %d", len(fl2.TrackCoordinates))
	}
	if fl2.TrackCoordinates[1][0] != -25.5958 || fl2.TrackCoordinates[1][1] != -54.4875 {
		t.Errorf("unexpected coord[1]: %v", fl2.TrackCoordinates[1])
	}

	// Out of bounds lat/lon filtered out
	payloadOOB := `{"id":3,"track_coordinates":[[-95.0, 10.0],[10.0, 200.0],[10.0, 20.0]]}`
	var w3 flightWire
	if err := json.Unmarshal([]byte(payloadOOB), &w3); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	fl3, err := w3.toDomain()
	if err != nil {
		t.Fatalf("toDomain error: %v", err)
	}
	if len(fl3.TrackCoordinates) != 1 {
		t.Fatalf("expected 1 valid coordinate after filtering out-of-bounds, got %d", len(fl3.TrackCoordinates))
	}
	if fl3.TrackCoordinates[0][0] != 10.0 || fl3.TrackCoordinates[0][1] != 20.0 {
		t.Errorf("unexpected coord: %v", fl3.TrackCoordinates[0])
	}
}

func TestFlightWireLenientWithUnexpectedRouteAndTrackShapes(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{
			name:    "route is an object instead of string",
			payload: `{"id":10,"route":{"waypoints":["BITIS","KASUK"]}}`,
		},
		{
			name:    "track_coordinates is a string instead of array",
			payload: `{"id":11,"track_coordinates":"invalid-string"}`,
		},
		{
			name:    "track_coordinates is an object instead of array",
			payload: `{"id":12,"track_coordinates":{"points":[[1,2]]}}`,
		},
		{
			name:    "track_coordinates is a flat array of numbers",
			payload: `{"id":13,"track_coordinates":[12.3, 45.6]}`,
		},
		{
			name:    "track_coordinates is null",
			payload: `{"id":14,"track_coordinates":null,"route":null}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w flightWire
			if err := json.Unmarshal([]byte(tc.payload), &w); err != nil {
				t.Fatalf("flightWire unmarshal must not fail for unexpected shape: %v", err)
			}
			fl, err := w.toDomain()
			if err != nil {
				t.Fatalf("toDomain must not fail for unexpected shape: %v", err)
			}
			if fl.ID != "10" && fl.ID != "11" && fl.ID != "12" && fl.ID != "13" && fl.ID != "14" {
				t.Errorf("unexpected flight id: %q", fl.ID)
			}
		})
	}
}

func TestGetMyFlightsDoesNotDropFlightsOnUnexpectedTrackOrRoute(t *testing.T) {
	payload := `{
		"data": [
			{
				"id": 201,
				"callsign": "GLO201",
				"departure_airport_icao": "SBGR",
				"arrival_airport_icao": "SBRJ",
				"route": {"unexpected": "object"},
				"track_coordinates": [{"lat": -23.4, "lon": -46.4}]
			},
			{
				"id": 202,
				"callsign": "GLO202",
				"departure_airport_icao": "SBRJ",
				"arrival_airport_icao": "SBBR",
				"route": "DIRECT",
				"track_coordinates": "unexpected-string"
			}
		],
		"meta": {"total": 2, "current_page": 1, "last_page": 1}
	}`

	api := &flightLogAPI{stubAPI: stubAPI{
		status: 200,
		body:   []byte(payload),
	}}
	a := &App{Airspace: api}

	res, err := a.GetMyFlights(1, 10)
	if err != nil {
		t.Fatalf("GetMyFlights unexpected error: %v", err)
	}
	if len(res.Flights) != 2 {
		t.Fatalf("expected 2 flights, got %d (logbook must not be emptied by unexpected server shapes)", len(res.Flights))
	}
	if len(res.Flights[0].TrackCoordinates) != 1 {
		t.Errorf("expected 1 track coordinate for flight 201, got %d", len(res.Flights[0].TrackCoordinates))
	}
	if res.Flights[1].Route != "DIRECT" {
		t.Errorf("expected route DIRECT for flight 202, got %q", res.Flights[1].Route)
	}
	if len(res.Flights[1].TrackCoordinates) != 0 {
		t.Errorf("expected 0 track coordinates for flight 202, got %d", len(res.Flights[1].TrackCoordinates))
	}
}
