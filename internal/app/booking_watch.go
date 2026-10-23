package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

// bookingRefreshEvery is how often an active flight re-reads its booking. A
// diversion is declared on the airline's website, not in the ACARS, and the
// server records it on the booking, so this is how the flight learns its new
// arrival airport. Half a minute is long before the aircraft can be parked
// at the alternate, for one small GET; the dashboard already makes the same
// request every ten seconds while no flight is active.
const bookingRefreshEvery = 30 * time.Second

// errOtherBooking means the booking reply was for a different booking than
// the active flight's: the flight's booking has gone and the pilot's next one
// came back in its place.
var errOtherBooking = errors.New("booking reply is for another booking")

// errNoArrival means the booking reply named no arrival airport, which leaves
// nothing to move the flight's arrival to.
var errNoArrival = errors.New("booking names no arrival airport")

// bookingWatcher keeps an active flight's arrival airport current. It
// re-reads the booking on its own goroutine for as long as the flight lasts
// and reports a change of arrival, so position reports, the finish request
// and the auto-finish check all follow a diversion. It also keeps the
// booking's departure, whose coordinates the flight was started without.
type bookingWatcher struct {
	bookingID string
	fetch     func() (activeBooking, error)
	// onArrival is told the booking's new arrival airport when it changes.
	// It is called on the watcher's goroutine, never with mu held.
	onArrival func(bookingID string, arrival domain.AirportInfo)
	every     time.Duration

	stopCh   chan struct{}
	stopOnce sync.Once

	// mu guards departure, arrival and failures.
	mu        sync.Mutex
	departure domain.AirportInfo
	arrival   domain.AirportInfo
	failures  int
}

// startBookingWatcher watches the active flight's booking, starting from the
// airports the flight was started with.
func (a *App) startBookingWatcher(bookingID, departure, arrival string) *bookingWatcher {
	w := &bookingWatcher{
		bookingID: bookingID,
		fetch:     a.fetchBooking,
		onArrival: a.applyArrival,
		every:     bookingRefreshEvery,
		stopCh:    make(chan struct{}),
		departure: airportFromICAO(departure),
		arrival:   airportFromICAO(arrival),
	}
	go w.run()
	return w
}

// Departure returns the airport the booking last named as the flight's
// departure, with its coordinates when the booking gave them.
func (w *bookingWatcher) Departure() domain.AirportInfo {
	if w == nil {
		return domain.AirportInfo{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.departure
}

// Arrival returns the airport the booking last named as the flight's
// arrival, with its coordinates when the booking gave them.
func (w *bookingWatcher) Arrival() domain.AirportInfo {
	if w == nil {
		return domain.AirportInfo{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.arrival
}

// Stop ends the watcher. It does not wait for the goroutine: it is called
// with flightMu held, and a refresh in progress may be waiting on that lock
// to apply its answer. That refresh sees the stop and drops the answer.
func (w *bookingWatcher) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() { close(w.stopCh) })
}

func (w *bookingWatcher) stopped() bool {
	select {
	case <-w.stopCh:
		return true
	default:
		return false
	}
}

// run refreshes at once, for the coordinates the flight was started without,
// then every w.every until stopped.
func (w *bookingWatcher) run() {
	defer observability.Recover()

	ticker := time.NewTicker(w.every)
	defer ticker.Stop()
	for {
		w.refresh()
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
		}
	}
}

// refresh reads the booking once. A failed read keeps the last known
// arrival: the flight is still going where it was going.
func (w *bookingWatcher) refresh() {
	_, span := observability.Start(context.Background(), "booking.refresh",
		"flight.booking_id", w.bookingID)
	defer span.Finish()

	b, err := w.fetch()
	if err == nil && b.ID != w.bookingID {
		err = errOtherBooking
	}
	if err == nil && b.Arrival.ICAO == "" {
		err = errNoArrival
	}
	if err != nil {
		span.Expected(err)
		w.noteFailure(err)
		return
	}

	w.mu.Lock()
	previous := w.arrival
	w.arrival = b.Arrival
	if b.Departure.ICAO != "" {
		w.departure = b.Departure
	}
	w.failures = 0
	w.mu.Unlock()

	if w.stopped() || strings.EqualFold(previous.ICAO, b.Arrival.ICAO) {
		return
	}
	span.Set("flight.arrival", b.Arrival.ICAO)
	w.onArrival(w.bookingID, b.Arrival)
}

// noteFailure logs a failed refresh: the first of a streak at Warn, unless it
// is only the network, and the rest at Debug, since they come every
// bookingRefreshEvery for as long as the cause lasts.
func (w *bookingWatcher) noteFailure(err error) {
	w.mu.Lock()
	w.failures++
	n := w.failures
	w.mu.Unlock()

	if n == 1 && !observability.Transient(err) {
		slog.Warn("booking refresh failed; keeping the last known arrival airport",
			"flight.booking_id", w.bookingID, "error", err)
		return
	}
	slog.Debug("booking refresh failed",
		"flight.booking_id", w.bookingID, "error", err, "failures", n)
}

// applyArrival moves the active flight's arrival to the airport its booking
// now names. A watcher left over from a flight that has since ended finds the
// flight idle, or on another booking, and changes nothing.
func (a *App) applyArrival(bookingID string, arrival domain.AirportInfo) {
	a.flightMu.Lock()
	if a.state == "idle" || a.bookingID != bookingID || strings.EqualFold(a.arrival, arrival.ICAO) {
		a.flightMu.Unlock()
		return
	}
	from := a.arrival
	a.arrival = arrival.ICAO
	a.flightMu.Unlock()

	observability.Count("flight.arrival_changed")
	slog.Info("flight arrival changed", "flight.arrival_from", from, "flight.arrival", arrival.ICAO)
	a.UI.EmitEvent("flight-arrival-changed", arrival.ICAO)
}

// flightArrival is the active flight's arrival airport, with the coordinates
// the booking gave it when the watcher holds them for that same airport.
func flightArrival(icao string, w *bookingWatcher) domain.AirportInfo {
	if watched := w.Arrival(); watched.ICAO != "" && strings.EqualFold(watched.ICAO, icao) {
		return watched
	}
	return airportFromICAO(icao)
}

// flightDeparture is the active flight's departure airport, with the
// coordinates the booking gave it when the watcher holds them for that same
// airport. The flight is started with only the departure's code, and the
// built-in table places a small set of airports.
func flightDeparture(icao string, w *bookingWatcher) domain.AirportInfo {
	if watched := w.Departure(); watched.ICAO != "" && strings.EqualFold(watched.ICAO, icao) {
		return watched
	}
	return airportFromICAO(icao)
}
