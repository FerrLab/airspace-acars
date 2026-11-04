package app

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

// autoAction is what one reading of the simulator calls for.
type autoAction int

const (
	autoNone autoAction = iota
	// autoStart: the aircraft has just become ready to depart. Whether it is
	// at the departure airport is known only once the booking is fetched.
	autoStart
	// autoFinish: the aircraft has just been shut down at its arrival
	// airport after flying.
	autoFinish
	// autoFinishSkipped: the aircraft has just been shut down after flying,
	// but not at its arrival airport.
	autoFinishSkipped
)

// autoTick is what deciding on one reading needs to know.
type autoTick struct {
	fd       *domain.FlightData
	startOn  bool // the pilot's auto-start setting
	finishOn bool // the pilot's auto-finish setting
	state    string
	// started identifies the active flight: its start time.
	started time.Time
	// arrival is the active flight's arrival airport, and simAirport the
	// field the simulator's scenery puts the aircraft on, if any.
	arrival    domain.AirportInfo
	simAirport string
}

// autoFlight decides when a flight starts or finishes without the pilot
// pressing a button. It is handed each reading and answers with at most one
// action per occasion: one start attempt each time the aircraft becomes ready
// to depart, one finish each time it is shut down at its arrival airport.
// Holding the edge matters: the conditions last for minutes, and the reading
// comes every second.
type autoFlight struct {
	// mu guards every field. The data stream calls step from one goroutine,
	// but a reconnect starts the next stream before the last has
	// necessarily returned.
	mu sync.Mutex

	// startArmed is set when a start has been attempted for the current
	// readiness to depart, and cleared when the aircraft stops being ready.
	startArmed bool

	// flight is the start time of the flight the fields below describe; a
	// different one resets them.
	flight time.Time
	// airborne is set once the flight has left the ground. Until then a
	// shutdown is a pause at the gate, which matters most for a circuit,
	// whose arrival airport is the one it is parked at.
	airborne bool
	// finishArmed is set when a finish has been attempted for the current
	// shutdown, and skipNoted when the pilot has been told the shutdown is
	// not at the arrival airport. Both clear when the engines or beacon
	// come back on.
	finishArmed bool
	skipNoted   bool
}

// step decides what one reading calls for.
func (f *autoFlight) step(t autoTick) autoAction {
	if t.fd == nil {
		return autoNone
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	switch t.state {
	case "idle":
		return f.stepIdleLocked(t)
	case "active":
		return f.stepActiveLocked(t)
	}
	// Finishing: the outcome is up to the drain, and a failed finish returns
	// to active with the shutdown still armed, so it is not retried every
	// second.
	return autoNone
}

// stepIdleLocked arms a start. Callers hold f.mu.
func (f *autoFlight) stepIdleLocked(t autoTick) autoAction {
	if !t.startOn {
		return autoNone
	}
	if !domain.ReadyToDepart(t.fd) {
		f.startArmed = false
		return autoNone
	}
	if f.startArmed {
		return autoNone
	}
	f.startArmed = true
	return autoStart
}

// stepActiveLocked follows the flight and arms a finish. Callers hold f.mu.
func (f *autoFlight) stepActiveLocked(t autoTick) autoAction {
	if !t.started.Equal(f.flight) {
		f.flight = t.started
		f.airborne, f.finishArmed, f.skipNoted = false, false, false
	}
	// Followed whatever the setting, so that turning auto-finish on during
	// the cruise still finishes this flight.
	if !t.fd.Sensors.OnGround {
		f.airborne = true
	}
	if !t.finishOn {
		return autoNone
	}
	if !f.airborne || !domain.ShutDown(t.fd) {
		f.finishArmed, f.skipNoted = false, false
		return autoNone
	}
	if f.finishArmed {
		return autoNone
	}
	if domain.AtAirport(t.fd.Position.Latitude, t.fd.Position.Longitude, t.arrival, t.simAirport) {
		f.finishArmed = true
		return autoFinish
	}
	// Not armed: a diversion the booking has yet to report would put the
	// arrival here on a later reading, and the flight should finish then.
	if f.skipNoted {
		return autoNone
	}
	f.skipNoted = true
	return autoFinishSkipped
}

// checkAutoFlight starts or finishes the flight on its own when a reading
// calls for it. Called once per tick from dataStreamLoop.
func (a *App) checkAutoFlight(data *domain.FlightData) {
	settings := a.GetSettings()
	if settings.LocalMode || data == nil {
		return
	}

	a.flightMu.Lock()
	state, started, arrival, watch := a.state, a.startTime, a.arrival, a.bookingWatch
	a.flightMu.Unlock()

	lat, lon := data.Position.Latitude, data.Position.Longitude
	t := autoTick{
		fd:       data,
		startOn:  settings.AutoStartFlight,
		finishOn: settings.AutoFinishFlight,
		state:    state,
		started:  started,
	}
	if state == "active" {
		t.arrival = flightArrival(arrival, watch)
		t.simAirport = a.currentLocator().AirportAt(lat, lon)
	}

	switch a.auto.step(t) {
	case autoStart:
		go a.tryAutoStartFlight(lat, lon)
	case autoFinish:
		a.autoFinishFlight(t.arrival.ICAO)
	case autoFinishSkipped:
		slog.Info("auto-finish: shut down away from the arrival airport; leaving the flight open",
			"flight.arrival", t.arrival.ICAO, "sim.airport", t.simAirport)
		a.UI.EmitEvent("auto-flight-skipped",
			domain.AutoFlightSkip{Action: "finish", Airport: t.arrival.ICAO})
	}
}

// tryAutoStartFlight fetches the booking and starts it, if the aircraft at
// (lat, lon) is at the booking's departure airport.
func (a *App) tryAutoStartFlight(lat, lon float64) {
	defer observability.Recover()

	// Before a tenant is picked (or after sign-out) the sim can already be
	// ready; asking for the booking then fails inside the adapter and is
	// reported as a fault (Sentry AIRSPACE-ACARS-K). There is nothing to start.
	if a.Airspace.BaseURL() == "" {
		slog.Debug("auto-start: no tenant selected")
		return
	}

	booking, err := a.fetchBooking()
	if errors.Is(err, errNoBooking) {
		slog.Debug("auto-start: no booking available")
		return
	}
	if err != nil {
		slog.Debug("auto-start: failed to fetch booking", "error", err)
		return
	}
	if booking.Departure.ICAO == "" {
		slog.Debug("auto-start: the booking names no departure airport")
		return
	}

	simAirport := a.currentLocator().AirportAt(lat, lon)
	if !domain.AtAirport(lat, lon, booking.Departure, simAirport) {
		slog.Info("auto-start: not at the departure airport; leaving the start to the pilot",
			"flight.departure", booking.Departure.ICAO, "sim.airport", simAirport)
		a.UI.EmitEvent("auto-flight-skipped",
			domain.AutoFlightSkip{Action: "start", Airport: booking.Departure.ICAO})
		return
	}

	a.UI.EmitEvent("auto-flight-start", booking.Callsign)
	if err := a.StartFlight(booking.Callsign, booking.Departure.ICAO, booking.Arrival.ICAO, booking.ID); err != nil {
		slog.Warn("auto-start: failed to start flight", "error", err)
		return
	}
	slog.Info("auto-start: flight started", "callsign", booking.Callsign,
		"dep", booking.Departure.ICAO, "arr", booking.Arrival.ICAO)
}

// autoFinishFlight finishes the active flight, parked at arrival.
func (a *App) autoFinishFlight(arrival string) {
	if err := a.FinishFlight(); err != nil {
		slog.Warn("auto-finish: failed to finish flight", "error", err)
		return
	}
	slog.Info("auto-finish: flight finishing", "flight.arrival", arrival)
	a.UI.EmitEvent("auto-flight-finish", arrival)
}
