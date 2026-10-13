package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"airspace-acars/internal/domain"
)

const bookingPath = "/api/v2/acars/booking"

// errNoBooking means the booking reply named no flight: the pilot has not
// booked one, or the one they had is gone.
var errNoBooking = errors.New("no booking")

// activeBooking is the part of the pilot's booking the flight lifecycle reads.
type activeBooking struct {
	ID        string
	Callsign  string
	Departure domain.AirportInfo
	// Arrival is where the flight is going now: the alternate once the pilot
	// has diverted to one, the booked arrival otherwise.
	Arrival domain.AirportInfo
}

type bookingWire struct {
	ID               json.RawMessage    `json:"id"`
	Callsign         string             `json:"callsign"`
	FlightNumber     string             `json:"flight_number"`
	DepartureAirport *flightWireAirport `json:"departure_airport"`
	ArrivalAirport   *flightWireAirport `json:"arrival_airport"`
	AlternateAirport *flightWireAirport `json:"alternate_airport"`
}

// parseBooking decodes the booking reply. The server sets alternate_airport
// when the pilot diverts, and from then on that is the flight's arrival, as
// the dashboard has always shown it. Airport coordinates may come as
// latitude/longitude or lat/lon, numbers or strings, as the logbook's do.
func parseBooking(body []byte) (activeBooking, error) {
	var w bookingWire
	if err := json.Unmarshal(body, &w); err != nil {
		return activeBooking{}, fmt.Errorf("parse booking: %w", err)
	}
	id, err := parseRawID(w.ID)
	if err != nil {
		return activeBooking{}, fmt.Errorf("parse booking id: %w", err)
	}
	callsign := strings.TrimSpace(w.Callsign)
	if callsign == "" {
		callsign = strings.TrimSpace(w.FlightNumber)
	}
	if callsign == "" {
		return activeBooking{}, errNoBooking
	}

	b := activeBooking{ID: id, Callsign: callsign}
	if w.DepartureAirport != nil {
		b.Departure = w.DepartureAirport.toDomain()
	}
	if w.AlternateAirport != nil {
		b.Arrival = w.AlternateAirport.toDomain()
	}
	if b.Arrival.ICAO == "" && w.ArrivalAirport != nil {
		b.Arrival = w.ArrivalAirport.toDomain()
	}
	return b, nil
}

// fetchBooking reads the pilot's current booking from the tenant.
func (a *App) fetchBooking() (activeBooking, error) {
	body, status, err := a.Airspace.DoRequest("GET", bookingPath, nil)
	if err != nil {
		return activeBooking{}, fmt.Errorf("fetch booking: %w", err)
	}
	if err := domain.NewStatusError("GET", bookingPath, status, body); err != nil {
		return activeBooking{}, err
	}
	return parseBooking(body)
}

// airportFromICAO stands in for an airport known only by its code, placed
// from the built-in table of coordinates when it is one of the airports
// listed there.
func airportFromICAO(icao string) domain.AirportInfo {
	icao = strings.ToUpper(strings.TrimSpace(icao))
	lat, lon := resolveCoordinates(icao, 0, 0)
	return domain.AirportInfo{ICAO: icao, Latitude: lat, Longitude: lon}
}
