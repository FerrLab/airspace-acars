package app

import "airspace-acars/internal/domain"

// attendantSteps is the phase map in flight order. Meal services are added
// after cruise, one per service the pack plans.
var attendantSteps = []string{
	domain.PhaseBoarding,
	domain.PhaseBoardingComplete,
	domain.PhasePushback,
	domain.PhaseTaxi,
	domain.PhaseClimbOut,
	domain.PhasePassing10000,
	domain.PhaseCruise,
	domain.PhaseMealService,
	domain.PhaseSeatbeltOff,
	domain.PhaseSeatbeltOn,
	domain.PhaseDescend,
	domain.PhaseDescending10000,
	domain.PhaseFinalApproach,
	domain.PhaseAfterLanding,
	domain.PhaseGateArrival,
	domain.PhaseAlternate,
}

// Snapshot is everything the Flight Attendant tab shows.
func (e *attendant) Snapshot() domain.AttendantSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()

	snap := domain.AttendantSnapshot{
		Enabled:  e.enabled,
		Mode:     e.mode,
		Current:  e.current,
		Arrival:  e.packArrival,
		Steps:    []domain.AttendantStep{},
		Queue:    []domain.AttendantClip{},
		History:  []domain.AttendantHistoryEntry{},
		GSX:      e.gsxState,
		Seatbelt: e.seatbelt,
		Error:    e.loadErr,
		Seatbelts: domain.SeatbeltCounts{
			On:  e.counts[domain.PhaseSeatbeltOn],
			Off: e.counts[domain.PhaseSeatbeltOff],
		},
	}
	if e.preparing != nil {
		p := *e.preparing
		snap.Preparing = &p
	}
	if e.pack != nil && e.pack.Flight.Arrival != "" {
		snap.Arrival = e.pack.Flight.Arrival
	}
	if e.playing != nil {
		c := e.playing.AttendantClip
		snap.NowPlaying = &c
	}
	for _, c := range e.queue {
		snap.Queue = append(snap.Queue, c.AttendantClip)
	}
	// Newest first, as the tab lists it.
	for i := len(e.history) - 1; i >= 0; i-- {
		snap.History = append(snap.History, e.history[i])
	}
	if e.detector == nil && e.pack == nil {
		return snap
	}
	for _, phase := range attendantSteps {
		if phase != domain.PhaseMealService {
			snap.Steps = append(snap.Steps, e.stepLocked(phase, 0))
			continue
		}
		if e.pack == nil {
			continue
		}
		for _, m := range e.pack.Flight.MealServices {
			snap.Steps = append(snap.Steps, e.stepLocked(phase, m.N))
		}
	}
	return snap
}

// stepLocked is one marker of the phase map. Callers hold e.mu.
func (e *attendant) stepLocked(phase string, service int) domain.AttendantStep {
	key := stepKeyFor(phase, service)
	step := domain.AttendantStep{Key: key, Phase: phase, Service: service, Status: domain.StepPending,
		Count: e.counts[key], Reason: e.reasons[key]}
	if e.pack != nil {
		for _, l := range e.pack.playable(phase) {
			if service == 0 || l.service() == service {
				step.Lines++
			}
		}
	}
	if d := e.detector; d != nil {
		if at, ok := d.fired[key]; ok {
			step.Status, step.FiredAt = domain.StepFired, at
		} else if d.skipped[key] {
			step.Status = domain.StepSkipped
		}
	}
	return step
}
