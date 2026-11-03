package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

// load reads the pack and decides the mode.
func (e *attendant) load(gen int) (retry bool) {
	e.mu.Lock()
	arrival, bookingID := e.packArrival, e.bookingID
	e.mu.Unlock()

	_, span := observability.Start(context.Background(), "attendant.load_pack", "flight.booking_id", bookingID)
	defer span.Finish()
	pack, err := fetchSpeechPack(e.deps.api, arrival)

	e.mu.Lock()
	if e.gen != gen {
		e.mu.Unlock()
		return false
	}
	switch {
	case errors.Is(err, errSpeechUnsupported):
		span.Expected(err)
		e.mode, e.detector, e.loadErr, e.current = domain.AttendantLegacy, nil, "", ""
		slog.Info("attendant: the airline's server plays announcements itself; using the legacy player")
	case errors.Is(err, errNoBooking):
		span.Expected(err)
		e.mode, e.detector, e.loadErr, e.current = domain.AttendantNoBooking, nil, "", ""
		slog.Info("attendant: no active booking; no announcements this flight")
	case err != nil:
		e.loadErr = err.Error()
		e.loadFailures++
		// Retried every packRetryEvery for the whole flight, so only the
		// first failure of a streak is worth reporting.
		if observability.Transient(err) || e.loadFailures > 1 {
			span.Expected(err)
			slog.Debug("attendant: speech pack unavailable; will retry", "error", err)
		} else {
			span.Fail(err)
			slog.Warn("attendant: speech pack could not be read; will retry", "error", err)
		}
		retry = true
	default:
		span.Set("attendant.revision", pack.Revision)
		e.installPackLocked(pack, arrival)
		e.mode, e.loadErr, e.loadFailures = domain.AttendantClient, "", 0
		slog.Info("attendant: speech pack loaded", "attendant.revision", pack.Revision,
			"attendant.phases", len(pack.Phases))
	}
	e.mu.Unlock()
	e.notify()
	return retry
}

// installPackLocked makes pack the flight's pack: the detector learns the
// flight's timings and the audio is queued for download, in flight order.
// Callers hold e.mu.
func (e *attendant) installPackLocked(pack *speechPack, arrival string) {
	e.pack, e.packArrival = pack, arrival
	e.prepared = map[string][]domain.SoundInstruction{}
	e.prefetch = nil
	for _, ph := range pack.Phases {
		for _, l := range pack.playable(ph.Phase) {
			if !l.Dynamic && l.hasAudio() {
				e.prefetch = append(e.prefetch, l)
			}
		}
	}

	cfg := phaseConfig{
		start:        e.start,
		plannedBlock: time.Duration(float64(pack.Flight.PlannedBlockMinutes) * float64(time.Minute)),
		meals:        pack.Flight.MealServices,
		landing:      pack.Settings.Landing,
		seatbeltCooldown: map[string]time.Duration{
			domain.PhaseSeatbeltOn:  minutes(pack.Settings.Seatbelt.OnCooldownMinutes),
			domain.PhaseSeatbeltOff: minutes(pack.Settings.Seatbelt.OffCooldownMinutes),
		},
	}
	seen := map[int]bool{}
	for _, l := range pack.playable(domain.PhaseCruise) {
		if p := l.cruiseAt(); p > 0 && !seen[p] {
			seen[p] = true
			cfg.cruiseAt = append(cfg.cruiseAt, p)
		}
	}
	if e.detector != nil {
		e.detector.cfg = cfg
	}
}

func minutes(f flexFloat) time.Duration { return time.Duration(float64(f) * float64(time.Minute)) }

// divert reads the pack again for the new arrival, drops the audio prepared
// for the old one, and announces the diversion.
func (e *attendant) divert(gen int, icao string) {
	_, span := observability.Start(context.Background(), "attendant.divert", "flight.arrival", icao)
	defer span.Finish()

	pack, err := fetchSpeechPack(e.deps.api, icao)
	now := e.deps.now()

	e.mu.Lock()
	if e.gen != gen {
		e.mu.Unlock()
		return
	}
	if err != nil {
		// The lines still name the old destination, but the cabin should
		// hear about the diversion all the same.
		span.Expected(err)
		slog.Warn("attendant: speech pack for the alternate could not be read", "error", err)
	} else {
		e.installPackLocked(pack, icao)
		e.mode = domain.AttendantClient
	}
	f := phaseFire{phase: domain.PhaseAlternate, reason: domain.AttendantReason{Code: domain.ReasonDiversion, Airport: icao}}
	if e.detector != nil {
		e.detector.external(f.stepKey(), now)
	}
	e.firedLocked(f, now)
	e.mu.Unlock()
	e.notify()
}
