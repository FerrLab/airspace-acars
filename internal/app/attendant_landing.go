package app

import (
	"math"
	"time"

	"airspace-acars/internal/domain"
)

// touchdown is the landing being rolled out.
type touchdown struct {
	at     time.Time
	vs     float64
	g      float64
	bounce time.Time // when it left the ground again, zero while it has not
}

// landed is the first reading on the ground after flying: it takes the
// touchdown's rate from the sink just before it and starts the rollout.
func (d *phaseDetector) landed(s phaseSample, out []phaseFire) []phaseFire {
	d.airborne = time.Time{}
	if d.touchdown != nil {
		// A bounce: the first contact is the one the cabin felt most, and
		// its rate stands; the load keeps growing below.
		d.touchdown.bounce = time.Time{}
		return out
	}
	// The rate is the last reading in the air, the one closest to the
	// wheels touching: in the flare the sampler reads thirty times a
	// second, otherwise it is at most a second old. Earlier readings are
	// the approach, not the touchdown. The ground reading itself counts when
	// it is the steeper, as MSFS can still carry the impact in it.
	td := &touchdown{at: s.at, vs: s.fd.Attitude.VS}
	for i := len(d.recent) - 1; i >= 0; i-- {
		if r := d.recent[i]; !r.onGround {
			td.vs = math.Min(td.vs, r.vs)
			break
		}
	}
	d.touchdown = td
	d.skip(domain.PhaseClimbOut, domain.PhasePassing10000, domain.PhaseCruise, domain.PhaseDescend,
		domain.PhaseDescending10000, domain.PhaseFinalApproach)
	for _, pct := range d.cfg.cruiseAt {
		d.skip(cruiseAtKey(pct))
	}
	for _, m := range d.cfg.meals {
		d.skip(stepKeyFor(domain.PhaseMealService, m.N))
	}
	return out
}

// rollout follows the landing until the aircraft has slowed to taxi speed,
// keeping the largest load seen around the touchdown, then announces it.
func (d *phaseDetector) rollout(s phaseSample, out []phaseFire) []phaseFire {
	td := d.touchdown
	if s.at.Sub(td.at) <= touchdownWindow {
		for _, r := range d.recent {
			// The flare's own pull-up is not the landing.
			if !r.at.Before(td.at) && r.g > td.g {
				td.g = r.g
			}
		}
	}
	if s.fd.Attitude.GS >= rolloutEndKt {
		return out
	}
	class := domain.ClassifyLanding(td.vs, td.g, d.cfg.landing)
	d.touchdown = nil
	return d.fire(out, phaseFire{phase: domain.PhaseAfterLanding, landing: class, reason: domain.AttendantReason{
		Code: domain.ReasonTouchdown, VS: round(td.vs), G: math.Round(td.g*100) / 100, Landing: class}}, s.at)
}

func (d *phaseDetector) atArrival(s phaseSample, out []phaseFire) []phaseFire {
	if !d.has(domain.PhaseAfterLanding) {
		return out
	}
	switch {
	case s.gsx.Deboarding.State.InProgress():
		return d.fire(out, phaseFire{phase: domain.PhaseGateArrival,
			reason: domain.AttendantReason{Code: domain.ReasonGSXDeboarding}}, s.at)
	case !enginesRunning(s.fd) && s.fd.Attitude.GS < parkedMaxKt:
		return d.fire(out, phaseFire{phase: domain.PhaseGateArrival,
			reason: domain.AttendantReason{Code: domain.ReasonParked}}, s.at)
	}
	return out
}
