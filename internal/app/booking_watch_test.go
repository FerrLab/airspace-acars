package app

import (
	"errors"
	"net"
	"testing"
	"time"

	"airspace-acars/internal/domain"
)

// flyingApp is an App with flight AZU4512 under booking 42 in the air,
// booked to arrival.
func flyingApp(arrival string) (*App, *eventLog) {
	ui := newEventLog()
	return &App{UI: ui, state: "active", bookingID: "42", callsign: "AZU4512",
		departure: "SBGR", arrival: arrival}, ui
}

// watcherFor is a watcher for booking 42 that reads its booking from fetch.
func watcherFor(a *App, fetch func() (activeBooking, error)) *bookingWatcher {
	return &bookingWatcher{bookingID: "42", fetch: fetch, onArrival: a.applyArrival,
		every: time.Hour, stopCh: make(chan struct{}), arrival: airportFromICAO(a.arrival)}
}

func booked(id string, arrival domain.AirportInfo) func() (activeBooking, error) {
	return func() (activeBooking, error) {
		return activeBooking{ID: id, Callsign: "AZU4512", Departure: sbgr, Arrival: arrival}, nil
	}
}

// A diversion is declared on the website. The refresh moves the flight's
// arrival, which position reports, the finish request and auto-finish all
// read, and tells the dashboard.
func TestADiversionMovesTheFlightsArrival(t *testing.T) {
	a, ui := flyingApp("SBRJ")
	w := watcherFor(a, booked("42", sbsp))

	w.refresh()

	if got := a.GetActiveFlightInfo()["arrival"]; got != "SBSP" {
		t.Errorf("arrival = %q after diverting to SBSP; the flight would still finish at SBRJ", got)
	}
	if got := ui.sent("flight-arrival-changed"); len(got) != 1 || got[0] != "SBSP" {
		t.Errorf("flight-arrival-changed = %v, want [SBSP]", got)
	}
	if got := w.Arrival(); got != sbsp {
		t.Errorf("watched arrival = %+v, want %+v", got, sbsp)
	}
}

// The first refresh of an undiverted flight finds the arrival it started
// with. It changes nothing but the coordinates auto-finish places it by.
func TestARefreshOfTheSameArrivalOnlyAddsItsCoordinates(t *testing.T) {
	a, ui := flyingApp("SWYY")
	swyy := domain.AirportInfo{ICAO: "SWYY", Latitude: -11.2, Longitude: -56.1}
	w := watcherFor(a, booked("42", swyy))

	w.refresh()

	if got := ui.sent("flight-arrival-changed"); len(got) != 0 {
		t.Errorf("flight-arrival-changed = %v for an unchanged arrival", got)
	}
	if got := flightArrival("SWYY", w); got != swyy {
		t.Errorf("flight arrival = %+v, want the booking's %+v", got, swyy)
	}
}

// A reply for another booking, one with no arrival, or no reply at all,
// leaves the flight going where it was going.
func TestAFailedRefreshKeepsTheLastArrival(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fetch func() (activeBooking, error)
	}{
		{"another booking", booked("43", sbsp)},
		{"no arrival", booked("42", domain.AirportInfo{})},
		{"no booking", func() (activeBooking, error) { return activeBooking{}, errNoBooking }},
		{"offline", func() (activeBooking, error) {
			return activeBooking{}, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		}},
		{"server error", func() (activeBooking, error) {
			return activeBooking{}, domain.NewStatusError("GET", bookingPath, 502, nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, ui := flyingApp("SBRJ")
			w := watcherFor(a, tc.fetch)

			w.refresh()
			w.refresh()

			if got := a.GetActiveFlightInfo()["arrival"]; got != "SBRJ" {
				t.Errorf("arrival = %q, want SBRJ kept", got)
			}
			if got := w.Arrival(); got != sbrj {
				t.Errorf("watched arrival = %+v, want %+v kept", got, sbrj)
			}
			if got := ui.sent("flight-arrival-changed"); len(got) != 0 {
				t.Errorf("flight-arrival-changed = %v, want none", got)
			}
		})
	}
}

// A refresh that lands after its flight ended must not touch the next one.
func TestAWatcherFromAnEndedFlightChangesNothing(t *testing.T) {
	a, ui := flyingApp("SBRJ")
	w := watcherFor(a, booked("42", sbsp))
	a.state, a.bookingID, a.arrival = "active", "43", "SBKP" // the next flight

	w.refresh()

	if got := a.GetActiveFlightInfo()["arrival"]; got != "SBKP" {
		t.Errorf("the next flight's arrival = %q, want SBKP untouched", got)
	}
	if got := ui.sent("flight-arrival-changed"); len(got) != 0 {
		t.Errorf("flight-arrival-changed = %v, want none", got)
	}
}

// The watcher refreshes on its own until the flight ends, and ending the
// flight stops it without waiting on it.
func TestTheWatcherRefreshesUntilTheFlightEnds(t *testing.T) {
	a, _ := flyingApp("SBRJ")
	fetched := make(chan struct{}, 16)
	w := watcherFor(a, func() (activeBooking, error) {
		select {
		case fetched <- struct{}{}:
		default:
		}
		return activeBooking{ID: "42", Arrival: sbrj}, nil
	})
	w.every = time.Millisecond
	a.bookingWatch = w

	done := make(chan struct{})
	go func() { w.run(); close(done) }()
	for i := 0; i < 3; i++ {
		select {
		case <-fetched:
		case <-time.After(5 * time.Second):
			t.Fatalf("the booking was refreshed %d times, want it re-read every tick", i)
		}
	}

	a.flightMu.Lock()
	a.endFlight()
	a.flightMu.Unlock()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher kept running after its flight ended")
	}
	if a.bookingWatch != nil {
		t.Error("the ended flight still holds its watcher")
	}
}
