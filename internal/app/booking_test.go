package app

import (
	"errors"
	"testing"
)

// The booking reply is decoded once, here, into what the flight needs: its
// id, callsign, and where it departs from and is going now, with the
// coordinates auto-start and auto-finish place the aircraft against.
func TestParseBookingAcceptsTheServersVariants(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		want     activeBooking
		wantDepL [2]float64
		wantArrL [2]float64
	}{
		{
			name: "numeric id, numeric coordinates",
			body: `{"id": 42, "callsign": "AZU4512",
				"departure_airport": {"icao": "SBGR", "latitude": -23.43, "longitude": -46.47},
				"arrival_airport": {"icao": "SBRJ", "latitude": -22.91, "longitude": -43.16}}`,
			want:     activeBooking{ID: "42", Callsign: "AZU4512"},
			wantDepL: [2]float64{-23.43, -46.47},
			wantArrL: [2]float64{-22.91, -43.16},
		},
		{
			name: "string id, lat/lon as strings",
			body: `{"id": "b-7", "callsign": "AZU4512",
				"departure_airport": {"icao": "sbgr", "lat": "-23.43", "lon": "-46.47"},
				"arrival_airport": {"icao": "SBRJ", "lat": "-22.91", "lon": "-43.16"}}`,
			want:     activeBooking{ID: "b-7", Callsign: "AZU4512"},
			wantDepL: [2]float64{-23.43, -46.47},
			wantArrL: [2]float64{-22.91, -43.16},
		},
		{
			name: "flight number when there is no callsign",
			body: `{"id": 42, "flight_number": "4512",
				"departure_airport": {"icao": "SBGR"}, "arrival_airport": {"icao": "SBRJ"},
				"alternate_airport": null}`,
			want: activeBooking{ID: "42", Callsign: "4512"},
			// No coordinates in the reply: both come from the built-in table.
			wantDepL: [2]float64{-23.435556, -46.473056},
			wantArrL: [2]float64{-22.910278, -43.163056},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBooking([]byte(tt.body))
			if err != nil {
				t.Fatalf("parseBooking: %v", err)
			}
			if got.ID != tt.want.ID || got.Callsign != tt.want.Callsign {
				t.Errorf("id, callsign = %q, %q; want %q, %q", got.ID, got.Callsign, tt.want.ID, tt.want.Callsign)
			}
			if got.Departure.ICAO != "SBGR" || got.Arrival.ICAO != "SBRJ" {
				t.Errorf("route = %s-%s, want SBGR-SBRJ", got.Departure.ICAO, got.Arrival.ICAO)
			}
			if l := [2]float64{got.Departure.Latitude, got.Departure.Longitude}; l != tt.wantDepL {
				t.Errorf("departure at %v, want %v", l, tt.wantDepL)
			}
			if l := [2]float64{got.Arrival.Latitude, got.Arrival.Longitude}; l != tt.wantArrL {
				t.Errorf("arrival at %v, want %v", l, tt.wantArrL)
			}
		})
	}
}

// Once the pilot diverts, the server names the alternate on the booking, and
// that is where the flight is going.
func TestParseBookingTakesTheAlternateAsTheArrival(t *testing.T) {
	got, err := parseBooking([]byte(`{"id": 42, "callsign": "AZU4512",
		"departure_airport": {"icao": "SBGR"},
		"arrival_airport": {"icao": "SBRJ"},
		"alternate_airport": {"icao": "SBGL", "latitude": -22.81, "longitude": -43.25}}`))
	if err != nil {
		t.Fatalf("parseBooking: %v", err)
	}
	if got.Arrival.ICAO != "SBGL" || got.Arrival.Latitude != -22.81 {
		t.Errorf("arrival = %+v, want the alternate SBGL; the flight would finish at the airport it diverted away from", got.Arrival)
	}
}

func TestParseBookingRejectsGarbage(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want error
	}{
		{"no booking", `{}`, errNoBooking},
		{"null", `null`, errNoBooking},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseBooking([]byte(tt.body)); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
	for _, body := range []string{`<html>502</html>`, `{"id": {"x": 1}, "callsign": "A"}`, `[1, 2]`} {
		if _, err := parseBooking([]byte(body)); err == nil {
			t.Errorf("parseBooking(%s) accepted it", body)
		}
	}
}
