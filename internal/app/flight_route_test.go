package app

import (
	"errors"
	"testing"

	"airspace-acars/internal/domain"
)

// KDFW is not in the built-in airport table, so only the booking can place it.
var kdfw = domain.AirportInfo{ICAO: "KDFW", Name: "Dallas/Fort Worth", Latitude: 32.896828, Longitude: -97.037997}

// The progress bar measures the flight from the booking's departure. An
// airport outside the built-in table is placed by the booking's coordinates
// once the watcher has read them, instead of by wherever the aircraft
// happened to be when the dashboard first looked.
func TestTheRouteOfAFlightTakesTheDepartureFromItsBooking(t *testing.T) {
	a, _ := flyingApp("SBRJ")
	a.departure = "KDFW"
	w := watcherFor(a, func() (activeBooking, error) {
		return activeBooking{ID: "42", Callsign: "AZU4512", Departure: kdfw, Arrival: airportFromICAO("SBRJ")}, nil
	})
	a.bookingWatch = w

	before, err := a.GetFlightRoute()
	if err != nil {
		t.Fatal(err)
	}
	if before.Departure.Latitude != 0 || before.Departure.Longitude != 0 {
		t.Fatalf("departure placed at %+v before the booking was read; nothing knows where KDFW is yet", before.Departure)
	}

	w.refresh()

	route, err := a.GetFlightRoute()
	if err != nil {
		t.Fatal(err)
	}
	if route.Departure != kdfw {
		t.Errorf("departure = %+v, want the booking's %+v", route.Departure, kdfw)
	}
	if route.Arrival != airportFromICAO("SBRJ") {
		t.Errorf("arrival = %+v, want SBRJ", route.Arrival)
	}
}

// The watcher stores a diversion before the flight's arrival is moved to it.
// In between, the route must not pair the flight's arrival with the new
// airport's coordinates.
func TestTheRouteOnlyTakesTheWatchersAirportsWhenTheyMatchTheFlight(t *testing.T) {
	a, _ := flyingApp("SBRJ")
	w := watcherFor(a, booked("42", sbsp))
	w.arrival = sbsp
	a.bookingWatch = w

	route, err := a.GetFlightRoute()
	if err != nil {
		t.Fatal(err)
	}
	if route.Arrival != airportFromICAO("SBRJ") {
		t.Errorf("arrival = %+v while the flight still goes to SBRJ", route.Arrival)
	}
}

// Before a flight, the route is the pilot's booking, with the coordinates the
// tenant gives, and an alternate replaces the arrival as it does on the
// dashboard.
func TestTheRouteBeforeAFlightIsTheBooking(t *testing.T) {
	api := &routeAPI{}
	api.setBooking(`{"id": 42, "callsign": "AZU4512",
		"departure_airport": {"icao": "KDFW", "latitude": 32.896828, "longitude": -97.037997},
		"arrival_airport": {"icao": "KAUS", "latitude": 30.194533, "longitude": -97.669872},
		"alternate_airport": {"icao": "KIAH", "latitude": 29.984433, "longitude": -95.341442}}`)
	a := &App{Airspace: api, state: "idle"}

	route, err := a.GetFlightRoute()
	if err != nil {
		t.Fatal(err)
	}
	if route.Departure.ICAO != "KDFW" || route.Departure.Latitude != 32.896828 || route.Departure.Longitude != -97.037997 {
		t.Errorf("departure = %+v, want KDFW at the booking's coordinates", route.Departure)
	}
	if route.Arrival.ICAO != "KIAH" || route.Arrival.Latitude != 29.984433 {
		t.Errorf("arrival = %+v, want the alternate KIAH", route.Arrival)
	}
}

// With no booking the route is an error, not an empty route the dashboard
// would draw as a flight from (0, 0) to (0, 0).
func TestThereIsNoRouteWithoutABooking(t *testing.T) {
	a := &App{Airspace: &stubAPI{status: 404, body: []byte(`{"message":"No active booking"}`)}, state: "idle"}

	route, err := a.GetFlightRoute()

	var status *domain.StatusError
	if !errors.As(err, &status) || status.Status != 404 {
		t.Fatalf("GetFlightRoute() = %+v, %v; want the 404 as a StatusError", route, err)
	}
}
