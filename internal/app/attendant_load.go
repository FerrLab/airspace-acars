package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

// load reads the pack and decides the mode. It returns how long to wait
// before reading it again, 0 once the mode is decided.
func (e *attendant) load(gen int) (retryIn time.Duration) {
	e.mu.Lock()
	arrival, bookingID := e.packArrival, e.bookingID
	e.mu.Unlock()

	_, span := observability.Start(context.Background(), "attendant.load_pack", "flight.booking_id", bookingID)
	defer span.Finish()
	pack, err := fetchSpeechPack(e.deps.api, arrival)

	e.mu.Lock()
	if e.gen != gen {
		e.mu.Unlock()
		return 0
	}
	var preparing *packPreparing
	switch {
	case errors.Is(err, errSpeechUnsupported):
		span.Expected(err)
		e.mode, e.detector, e.loadErr, e.current = domain.AttendantLegacy, nil, "", ""
		e.preparing, e.early = nil, nil
		slog.Info("attendant: the airline's server plays announcements itself; using the legacy player")
	case errors.Is(err, errNoBooking):
		span.Expected(err)
		e.mode, e.detector, e.loadErr, e.current = domain.AttendantNoBooking, nil, "", ""
		e.preparing, e.early = nil, nil
		slog.Info("attendant: no active booking; no announcements this flight")
	case errors.As(err, &preparing):
		// The server is still voicing the pack: part of a normal start, so
		// no error on the tab and nothing counted against the server.
		span.Expected(err)
		e.loadErr = ""
		e.preparing = &domain.AttendantPreparing{LinesReady: preparing.linesReady, LinesTotal: preparing.linesTotal}
		slog.Debug("attendant: speech pack is being prepared; will ask again",
			"attendant.lines_ready", preparing.linesReady, "attendant.lines_total", preparing.linesTotal,
			"attendant.wait", preparing.retryAfter)
		retryIn = preparing.retryAfter
	case err != nil:
		e.loadErr, e.preparing = err.Error(), nil
		e.loadFailures++
		// Retried every packRetryEvery for the whole flight, so only the
		// first failure of a streak is worth reporting.
		if observability.Transient(err) || errors.Is(err, errEmptyPack) || e.loadFailures > 1 {
			span.Expected(err)
			slog.Debug("attendant: speech pack unavailable; will retry", "error", err)
		} else {
			span.Fail(err)
			slog.Warn("attendant: speech pack could not be read; will retry", "error", err)
		}
		retryIn = packRetryEvery
	default:
		span.Set("attendant.revision", pack.Revision)
		e.installPackLocked(pack, arrival)
		e.mode, e.loadErr, e.loadFailures, e.preparing = domain.AttendantClient, "", 0, nil
		slog.Info("attendant: speech pack loaded", "attendant.revision", pack.Revision,
			"attendant.phases", len(pack.Phases))
		e.playEarlyLocked()
	}
	e.mu.Unlock()
	e.notify()
	return retryIn
}

// holdUntilLoaded keeps a phase that fired before the pack was in, to play
// once it is. Callers do not hold e.mu.
func (e *attendant) holdUntilLoaded(gen int, f phaseFire, during string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.gen != gen || e.mode != domain.AttendantLoading {
		return
	}
	// The flight has left the step of anything held earlier: drop it, so
	// the list is no longer than one step's phases.
	kept := e.early[:0]
	for _, j := range e.early {
		if j.during == e.current {
			kept = append(kept, j)
		}
	}
	e.early = append(kept, attendantJob{gen: gen, kind: jobFire, fire: f, during: during})
}

// playEarlyLocked hands the worker the phases held while the pack loaded,
// those the flight is still at. Callers hold e.mu.
func (e *attendant) playEarlyLocked() {
	for _, j := range e.early {
		if j.during != e.current {
			slog.Info("attendant: phase given up; the flight moved on before the pack was in",
				"attendant.phase", j.fire.stepKey())
			continue
		}
		e.sendLocked(j)
	}
	e.early = nil
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
