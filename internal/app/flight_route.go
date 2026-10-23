package app

import (
	"fmt"

	"airspace-acars/internal/domain"
)

// GetFlightRoute returns the departure and arrival of the pilot's flight, with
// the coordinates the dashboard's progress bar measures by. During a flight
// they come from the booking as the booking watcher last read it, so a
// diversion moves the arrival; before one, from the pilot's current booking.
// An airport the booking does not place falls back to the built-in table, and
// has zero coordinates when the table does not know it either.
func (a *App) GetFlightRoute() (*domain.FlightRoute, error) {
	a.flightMu.Lock()
	inFlight := a.state == "active" || a.state == "finishing"
	departure, arrival, watch := a.departure, a.arrival, a.bookingWatch
	a.flightMu.Unlock()

	if inFlight {
		return &domain.FlightRoute{
			Departure: flightDeparture(departure, watch),
			Arrival:   flightArrival(arrival, watch),
		}, nil
	}

	b, err := a.fetchBooking()
	if err != nil {
		return nil, fmt.Errorf("flight route: %w", err)
	}
	return &domain.FlightRoute{Departure: b.Departure, Arrival: b.Arrival}, nil
}
