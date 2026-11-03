package app

import (
	"math"
	"strconv"
	"time"

	"airspace-acars/internal/domain"
)

// cruise fires reaching cruise, the cruise_at lines and the meal services.
func (d *phaseDetector) cruise(s phaseSample, out []phaseFire) []phaseFire {
	fd := s.fd
	alt, vs := fd.Position.Altitude, fd.Attitude.VS
	descending := d.has(domain.PhaseDescend)

	if !d.passed(domain.PhaseCruise) && !descending {
		if alt > tenThousandFt && math.Abs(vs) < cruiseLevelFPM {
			if d.levelSince.IsZero() {
				d.levelSince = s.at
			}
			if held := s.at.Sub(d.levelSince); held >= cruiseHold {
				d.cruiseAt, d.cruiseAlt = s.at, alt
				out = d.fire(out, phaseFire{phase: domain.PhaseCruise, reason: domain.AttendantReason{
					Code: domain.ReasonCruise, Altitude: round(alt), Seconds: int(held.Seconds())}}, s.at)
			}
		} else {
			d.levelSince = time.Time{}
		}
	}
	if !d.has(domain.PhaseCruise) {
		return out
	}

	for _, pct := range d.cfg.cruiseAt {
		key := cruiseAtKey(pct)
		if d.passed(key) || d.cfg.plannedBlock <= 0 {
			continue
		}
		due := d.cfg.start.Add(d.cfg.plannedBlock * time.Duration(pct) / 100)
		if s.at.Before(due) {
			continue
		}
		if descending {
			d.skip(key)
			continue
		}
		if alt < tenThousandFt || vs < -cruiseAtMaxSinkFPM {
			continue
		}
		out = d.fire(out, phaseFire{phase: domain.PhaseCruise, cruiseAt: pct, reason: domain.AttendantReason{
			Code: domain.ReasonCruiseAt, Percent: pct, Minutes: int(d.cfg.plannedBlock.Minutes())}}, s.at)
	}

	return d.meals(s, out, descending)
}

// meals fires the meal services: after reaching cruise, or before the
// estimated top of descent, and never once the descent has begun.
func (d *phaseDetector) meals(s phaseSample, out []phaseFire, descending bool) []phaseFire {
	for _, m := range d.cfg.meals {
		key := stepKeyFor(domain.PhaseMealService, m.N)
		if d.passed(key) {
			continue
		}
		if descending {
			d.skip(key)
			continue
		}
		minutes := time.Duration(float64(m.Minutes) * float64(time.Minute))
		var due time.Time
		code := domain.ReasonMealAfterCruise
		switch m.Timing {
		case "before_descent":
			if d.cfg.plannedBlock <= 0 {
				d.skip(key)
				continue
			}
			due = d.cfg.start.Add(d.cfg.plannedBlock - todBeforeBlockEnd - minutes)
			code = domain.ReasonMealBeforeDescent
		default:
			due = d.cruiseAt.Add(minutes)
		}
		if s.at.Before(due) {
			continue
		}
		out = d.fire(out, phaseFire{phase: domain.PhaseMealService, service: m.N, reason: domain.AttendantReason{
			Code: code, Service: m.N, Minutes: int(m.Minutes)}}, s.at)
	}
	return out
}

func cruiseAtKey(pct int) string { return domain.PhaseCruise + "@" + strconv.Itoa(pct) }

func (d *phaseDetector) descent(s phaseSample, out []phaseFire) []phaseFire {
	if d.passed(domain.PhaseDescend) || !d.passed(domain.PhaseClimbOut) {
		return out
	}
	fd := s.fd
	alt, vs := fd.Position.Altitude, fd.Attitude.VS
	below := d.peakAlt-alt >= descendBelowPeak
	if d.has(domain.PhaseCruise) {
		below = d.cruiseAlt-alt >= descendBelowCruise
	}
	if vs > -descendFPM || !below {
		d.descendSince = time.Time{}
		return out
	}
	if d.descendSince.IsZero() {
		d.descendSince = s.at
	}
	if s.at.Sub(d.descendSince) < descendHold {
		return out
	}
	d.skip(domain.PhaseCruise)
	feet := d.peakAlt - alt
	if d.has(domain.PhaseCruise) {
		feet = d.cruiseAlt - alt
	}
	return d.fire(out, phaseFire{phase: domain.PhaseDescend, reason: domain.AttendantReason{
		Code: domain.ReasonDescend, VS: round(vs), Altitude: round(feet)}}, s.at)
}
