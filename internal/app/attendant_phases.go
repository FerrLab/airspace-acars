package app

import (
	"math"
	"strconv"
	"time"

	"airspace-acars/internal/domain"
)

// The thresholds of phase detection. They follow the server's old detector
// where it had one and MSFS Universal Announcer (manual v0.9.9.7) where it
// did not, with the deviations each comment gives.
const (
	// pushbackMinKt is ground speed that counts as the aircraft moving with
	// the engines off: a tug pushing back. Under a knot is the sim settling.
	pushbackMinKt = 1.0
	// taxiMinKt is forward ground speed with an engine running that counts as
	// taxiing, high enough that a breakaway thrust nudge does not.
	taxiMinKt = 5.0
	// climbOutAfter and climbOutMinAGL time the after-takeoff announcement:
	// a minute after lift-off and clear of 1,000 ft, climbing. Universal
	// Announcer waits for 3,000 ft and two minutes; that is late for a
	// regional jet, and the crew is free to talk once the gear and flaps are
	// up.
	climbOutAfter  = 60 * time.Second
	climbOutMinAGL = 1_000.0
	// climbingFPM is the vertical speed that counts as climbing.
	climbingFPM = 100.0
	// tenThousandFt is the altitude the passing/descending announcements and
	// the cruise floor use.
	tenThousandFt = 10_000.0
	// cruiseLevelFPM and cruiseHold define reaching cruise: within 300 fpm of
	// level above 10,000 ft for two minutes. The server waited ten minutes,
	// which on a short sector is half the cruise; a minute, which the design
	// suggested, is the length of an ordinary level-off on the climb.
	cruiseLevelFPM = 300.0
	cruiseHold     = 2 * time.Minute
	// descendFPM, descendHold and descendBelowCruise define top of descent:
	// 500 fpm down for ten seconds and half a thousand feet off the cruise
	// level, so a step or a bump in the cruise is not a descent.
	descendFPM         = 500.0
	descendHold        = 10 * time.Second
	descendBelowCruise = 500.0
	// descendBelowPeak stands in for the cruise level on a flight that never
	// held one: a thousand feet off the highest point reached.
	descendBelowPeak = 1_000.0
	// cruiseAtMaxSinkFPM suppresses a 25/50/75 % cruise line while the
	// aircraft is descending faster than this (Universal Announcer's rule).
	cruiseAtMaxSinkFPM = 500.0
	// finalMaxAGL and finalMinSinkFPM time the cabin-crew-be-seated call:
	// gear down under 2,000 ft AGL, descending. Universal Announcer uses
	// 3,000 ft; 2,000 is closer to where a real crew is told to sit.
	finalMaxAGL     = 2_000.0
	finalMinSinkFPM = 300.0
	// rolloutEndKt is where the after-landing announcement waits for: the
	// rollout is over and the aircraft is turning off (Universal Announcer).
	rolloutEndKt = 15.0
	// touchdownWindow is how long after the touchdown its load is searched
	// for: the g spike comes a moment after the wheels touch. It is also how
	// far back the last reading in the air is looked for.
	touchdownWindow = 2 * time.Second
	// goAroundAfter is how long the aircraft has to be airborne again after
	// a touchdown for it to be a touch-and-go rather than a bounce.
	goAroundAfter = 5 * time.Second
	// parkedMaxKt is stopped at the gate.
	parkedMaxKt = 1.0
	// todBeforeBlockEnd estimates top of descent from the planned block time
	// for the before-descent meal service: half an hour before it ends.
	todBeforeBlockEnd = 30 * time.Minute
)

// phaseConfig is what the pack says about this flight.
type phaseConfig struct {
	start        time.Time
	plannedBlock time.Duration
	meals        []mealService
	// cruiseAt lists the percentages the pack has cruise lines for.
	cruiseAt         []int
	seatbeltCooldown map[string]time.Duration
	landing          domain.LandingThresholds
}

// phaseSample is one reading of the aircraft with its time.
type phaseSample struct {
	at  time.Time
	fd  *domain.FlightData
	gsx domain.GSXState
}

// phaseFire is a phase the detector has decided on.
type phaseFire struct {
	phase    string
	service  int    // meal service number
	cruiseAt int    // 0 on reaching cruise, else the percentage
	landing  string // after_landing's class
	reason   domain.AttendantReason
}

// stepKey is the phase map's key for a fire.
func (f phaseFire) stepKey() string {
	if f.phase == domain.PhaseCruise && f.cruiseAt > 0 {
		return cruiseAtKey(f.cruiseAt)
	}
	return stepKeyFor(f.phase, f.service)
}

func stepKeyFor(phase string, service int) string {
	if phase == domain.PhaseMealService && service > 0 {
		return phase + ":" + strconv.Itoa(service)
	}
	return phase
}

// recentSample is what the touchdown capture remembers of a reading.
type recentSample struct {
	at       time.Time
	vs, g    float64
	onGround bool
}

// phaseDetector decides, from the aircraft's readings, when each phase of
// the flight happens. It is pure: everything it knows comes in through step,
// with the time of the reading, so tests drive it through a whole flight
// without a clock or a sleep. It is not safe for concurrent use; the
// attendant calls it under its lock.
type phaseDetector struct {
	cfg phaseConfig

	fired   map[string]time.Time // step key → when
	skipped map[string]bool

	seen       bool // the first reading has been handled
	lastBeacon bool
	// lastParked is whether the previous reading was stopped on the ground
	// with every engine off: the server's old "Parked", which pushback is
	// the end of when GSX is not there to say so.
	lastParked   bool
	lastAlt      float64
	airborne     time.Time // when the current flight leg left the ground; zero on the ground
	everAirborne bool
	peakAlt      float64

	levelSince   time.Time
	cruiseAt     time.Time
	cruiseAlt    float64
	descendSince time.Time

	seatbeltKnown bool
	seatbelt      bool

	recent    []recentSample
	touchdown *touchdown
}

func newPhaseDetector(cfg phaseConfig) *phaseDetector {
	return &phaseDetector{cfg: cfg, fired: map[string]time.Time{}, skipped: map[string]bool{}}
}

func (d *phaseDetector) has(key string) bool {
	_, ok := d.fired[key]
	return ok
}

// passed reports whether a phase is behind the flight, fired or skipped.
func (d *phaseDetector) passed(key string) bool { return d.has(key) || d.skipped[key] }

func (d *phaseDetector) skip(keys ...string) {
	for _, k := range keys {
		if !d.has(k) {
			d.skipped[k] = true
		}
	}
}

// fire records a fire unless the phase already has one.
func (d *phaseDetector) fire(out []phaseFire, f phaseFire, at time.Time) []phaseFire {
	key := f.stepKey()
	if d.passed(key) {
		return out
	}
	d.fired[key] = at
	return append(out, f)
}

// external records a phase that something other than the readings decided,
// a diversion or a seatbelt change, so the phase map shows it.
func (d *phaseDetector) external(key string, at time.Time) {
	d.fired[key] = at
	delete(d.skipped, key)
}

// step takes one reading and returns the phases it fires, in flight order.
func (d *phaseDetector) step(s phaseSample) []phaseFire {
	fd := s.fd
	if fd == nil {
		return nil
	}
	var out []phaseFire
	if !d.seen {
		d.seen = true
		d.lastAlt = fd.Position.Altitude
		d.lastBeacon = fd.Lights.Beacon
		d.lastParked = parked(fd)
		d.seatbeltKnown, d.seatbelt = fd.Cabin.SeatbeltKnown, fd.Cabin.SeatbeltSign
		out = d.atStart(s, out)
	}

	d.remember(s)
	if fd.Sensors.OnGround {
		out = d.onGround(s, out)
	} else {
		out = d.inFlight(s, out)
	}
	out = d.seatbeltChange(s, out)

	d.lastAlt = fd.Position.Altitude
	d.lastBeacon = fd.Lights.Beacon
	d.lastParked = parked(fd)
	return out
}

// parked is the server's old "Parked": on the ground, stopped, every engine
// off.
func parked(fd *domain.FlightData) bool {
	return fd.Sensors.OnGround && fd.Attitude.GS < parkedMaxKt && !enginesRunning(fd)
}

// atStart fires boarding for a flight started at the gate. With GSX
// answering, boarding waits for GSX instead. Without it the old trigger
// holds: boarding plays when the flight starts, as it always has, whatever
// the beacon or the engines are doing.
func (d *phaseDetector) atStart(s phaseSample, out []phaseFire) []phaseFire {
	fd := s.fd
	if !fd.Sensors.OnGround {
		d.skip(domain.PhaseBoarding, domain.PhaseBoardingComplete, domain.PhasePushback, domain.PhaseTaxi)
		return out
	}
	if s.gsx.Connected {
		return out
	}
	return d.fire(out, phaseFire{phase: domain.PhaseBoarding,
		reason: domain.AttendantReason{Code: domain.ReasonFlightStarted}}, s.at)
}

// remember keeps the last few seconds of readings for the touchdown.
func (d *phaseDetector) remember(s phaseSample) {
	d.recent = append(d.recent, recentSample{at: s.at, vs: s.fd.Attitude.VS, g: s.fd.Attitude.GForce, onGround: s.fd.Sensors.OnGround})
	cut := 0
	for cut < len(d.recent) && s.at.Sub(d.recent[cut].at) > touchdownWindow {
		cut++
	}
	d.recent = d.recent[cut:]
}

func enginesRunning(fd *domain.FlightData) bool {
	for _, e := range fd.Engines {
		if e.Running {
			return true
		}
	}
	return false
}

func (d *phaseDetector) onGround(s phaseSample, out []phaseFire) []phaseFire {
	fd := s.fd
	if !d.airborne.IsZero() {
		out = d.landed(s, out)
	}
	if d.touchdown != nil {
		return d.rollout(s, out)
	}
	if d.everAirborne {
		return d.atArrival(s, out)
	}
	return d.atDeparture(s, out, fd)
}

func (d *phaseDetector) atDeparture(s phaseSample, out []phaseFire, fd *domain.FlightData) []phaseFire {
	if !s.gsx.Connected {
		return d.atDepartureWithoutGSX(s, out, fd)
	}
	gs := fd.Attitude.GS
	running := enginesRunning(fd)

	if !d.passed(domain.PhaseBoardingComplete) && s.gsx.Boarding.State == domain.GSXActive {
		out = d.fire(out, phaseFire{phase: domain.PhaseBoarding,
			reason: domain.AttendantReason{Code: domain.ReasonGSXBoarding}}, s.at)
	}

	var done *domain.AttendantReason
	switch {
	case s.gsx.Boarding.Completed && d.has(domain.PhaseBoarding):
		done = &domain.AttendantReason{Code: domain.ReasonGSXBoardingDone}
	case fd.Lights.Beacon && !d.lastBeacon:
		done = &domain.AttendantReason{Code: domain.ReasonBeaconOn}
	}

	var push *domain.AttendantReason
	switch {
	case s.gsx.Departure.State == domain.GSXActive:
		push = &domain.AttendantReason{Code: domain.ReasonGSXPushback}
	case gs > pushbackMinKt && !running:
		push = &domain.AttendantReason{Code: domain.ReasonMovingEnginesOff, GS: round(gs)}
	}
	taxiing := gs > taxiMinKt && running

	// Pushback or taxi with boarding still open closes it first: the
	// doors are shut by the time the aircraft moves.
	if done == nil && (push != nil || taxiing) && !d.passed(domain.PhaseBoardingComplete) {
		r := domain.AttendantReason{Code: domain.ReasonTaxi, GS: round(gs)}
		if push != nil {
			r = *push
		}
		done = &r
	}
	if done != nil && !d.passed(domain.PhaseBoardingComplete) {
		d.skip(domain.PhaseBoarding)
		out = d.fire(out, phaseFire{phase: domain.PhaseBoardingComplete, reason: *done}, s.at)
	}
	if push != nil && !d.passed(domain.PhaseTaxi) {
		out = d.fire(out, phaseFire{phase: domain.PhasePushback, reason: *push}, s.at)
	}
	if taxiing {
		d.skip(domain.PhasePushback)
		out = d.fire(out, phaseFire{phase: domain.PhaseTaxi,
			reason: domain.AttendantReason{Code: domain.ReasonTaxi, GS: round(gs)}}, s.at)
	}
	return out
}

// atDepartureWithoutGSX keeps the triggers the server used before the ACARS
// timed the announcements, for pilots who do not run GSX: pushback is the
// aircraft leaving "Parked", by creeping off the stand or by starting an
// engine while stopped; taxi is the first 5 kt; and boarding complete plays
// right before whichever of the two comes first, as the server plays it for
// older clients.
func (d *phaseDetector) atDepartureWithoutGSX(s phaseSample, out []phaseFire, fd *domain.FlightData) []phaseFire {
	gs := fd.Attitude.GS

	var push *domain.AttendantReason
	switch {
	case !d.lastParked || gs >= taxiMinKt:
	case gs >= pushbackMinKt:
		push = &domain.AttendantReason{Code: domain.ReasonLeftStand, GS: round(gs)}
	case enginesRunning(fd):
		push = &domain.AttendantReason{Code: domain.ReasonEngineStart}
	}
	taxiing := gs >= taxiMinKt

	if (push != nil || taxiing) && !d.passed(domain.PhaseBoardingComplete) {
		d.skip(domain.PhaseBoarding)
		out = d.fire(out, phaseFire{phase: domain.PhaseBoardingComplete,
			reason: domain.AttendantReason{Code: domain.ReasonLeavingGate}}, s.at)
	}
	if push != nil && !d.passed(domain.PhaseTaxi) {
		out = d.fire(out, phaseFire{phase: domain.PhasePushback, reason: *push}, s.at)
	}
	if taxiing {
		d.skip(domain.PhasePushback)
		out = d.fire(out, phaseFire{phase: domain.PhaseTaxi,
			reason: domain.AttendantReason{Code: domain.ReasonTaxi, GS: round(gs)}}, s.at)
	}
	return out
}

func (d *phaseDetector) inFlight(s phaseSample, out []phaseFire) []phaseFire {
	fd := s.fd
	alt, agl, vs := fd.Position.Altitude, fd.Position.AltitudeAGL, fd.Attitude.VS

	if d.airborne.IsZero() {
		d.airborne = s.at
		if d.touchdown != nil {
			d.touchdown.bounce = s.at
		}
	}
	if d.touchdown != nil {
		if s.at.Sub(d.touchdown.bounce) < goAroundAfter {
			return out
		}
		// Airborne long enough: a touch-and-go or a go-around, not a
		// bounce. The landing still to come is the one announced.
		d.touchdown = nil
	}
	if !d.everAirborne {
		d.everAirborne = true
		d.skip(domain.PhaseBoarding, domain.PhaseBoardingComplete, domain.PhasePushback, domain.PhaseTaxi)
	}
	if alt > d.peakAlt {
		d.peakAlt = alt
	}

	since := s.at.Sub(d.airborne)
	if since >= climbOutAfter && agl > climbOutMinAGL && vs > climbingFPM {
		out = d.fire(out, phaseFire{phase: domain.PhaseClimbOut, reason: domain.AttendantReason{
			Code: domain.ReasonClimbOut, Seconds: int(since.Seconds()), Altitude: round(agl)}}, s.at)
	}
	if d.lastAlt < tenThousandFt && alt >= tenThousandFt && vs > 0 {
		d.skip(domain.PhaseClimbOut)
		out = d.fire(out, phaseFire{phase: domain.PhasePassing10000,
			reason: domain.AttendantReason{Code: domain.ReasonPassing10000, VS: round(vs)}}, s.at)
	}

	out = d.cruise(s, out)
	out = d.descent(s, out)

	if d.lastAlt >= tenThousandFt && alt < tenThousandFt && vs < 0 &&
		(d.has(domain.PhaseCruise) || d.has(domain.PhaseDescend)) {
		out = d.fire(out, phaseFire{phase: domain.PhaseDescending10000,
			reason: domain.AttendantReason{Code: domain.ReasonDescending10000, VS: round(vs)}}, s.at)
	}
	if d.passed(domain.PhaseClimbOut) && agl < finalMaxAGL && vs < -finalMinSinkFPM && fd.Controls.GearDown {
		out = d.fire(out, phaseFire{phase: domain.PhaseFinalApproach, reason: domain.AttendantReason{
			Code: domain.ReasonFinalApproach, Altitude: round(agl), VS: round(vs)}}, s.at)
	}
	return out
}

// seatbeltChange fires the seatbelt announcements: in flight, after the
// climb-out, on a change of a sign the aircraft actually reports, and not
// again within the pack's cooldown for that announcement.
func (d *phaseDetector) seatbeltChange(s phaseSample, out []phaseFire) []phaseFire {
	cabin := s.fd.Cabin
	if !cabin.SeatbeltKnown {
		return out
	}
	was, wasKnown := d.seatbelt, d.seatbeltKnown
	d.seatbelt, d.seatbeltKnown = cabin.SeatbeltSign, true
	if !wasKnown || was == cabin.SeatbeltSign {
		return out
	}
	if s.fd.Sensors.OnGround || d.airborne.IsZero() || !d.passed(domain.PhaseClimbOut) {
		return out
	}
	phase, code := domain.PhaseSeatbeltOff, domain.ReasonSeatbeltOff
	if cabin.SeatbeltSign {
		phase, code = domain.PhaseSeatbeltOn, domain.ReasonSeatbeltOn
	}
	if last, ok := d.fired[phase]; ok && s.at.Sub(last) < d.cfg.seatbeltCooldown[phase] {
		return out
	}
	d.fired[phase] = s.at
	return append(out, phaseFire{phase: phase, reason: domain.AttendantReason{
		Code: code, Altitude: round(s.fd.Position.Altitude)}})
}

func round(f float64) int { return int(math.Round(f)) }
