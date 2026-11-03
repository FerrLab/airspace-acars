package app

import (
	"strings"
	"testing"
	"time"

	"airspace-acars/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var flightStart = time.Date(2026, 11, 3, 18, 0, 0, 0, time.UTC)

// flight drives a phaseDetector through a flight a reading at a time, on a
// clock it owns, and remembers what fired.
type flight struct {
	t     *testing.T
	d     *phaseDetector
	now   time.Time
	fd    domain.FlightData
	gsx   domain.GSXState
	fires []phaseFire
}

func newFlight(t *testing.T, cfg phaseConfig) *flight {
	cfg.start = flightStart
	if cfg.seatbeltCooldown == nil {
		cfg.seatbeltCooldown = map[string]time.Duration{
			domain.PhaseSeatbeltOn: 2 * time.Minute, domain.PhaseSeatbeltOff: 2 * time.Minute}
	}
	f := &flight{t: t, d: newPhaseDetector(cfg), now: flightStart}
	f.fd.Sensors.OnGround = true
	f.fd.Engines[0].Exists, f.fd.Engines[1].Exists = true, true
	return f
}

// hold feeds the current reading every second for d.
func (f *flight) hold(d time.Duration) *flight {
	for end := f.now.Add(d); !f.now.After(end); f.now = f.now.Add(time.Second) {
		f.fires = append(f.fires, f.d.step(phaseSample{at: f.now, fd: &f.fd, gsx: f.gsx})...)
	}
	return f
}

// keys lists what fired, in order.
func (f *flight) keys() []string {
	var out []string
	for _, fire := range f.fires {
		out = append(out, fire.stepKey())
	}
	return out
}

func (f *flight) last(phase string) phaseFire {
	f.t.Helper()
	for i := len(f.fires) - 1; i >= 0; i-- {
		if f.fires[i].phase == phase {
			return f.fires[i]
		}
	}
	f.t.Fatalf("%s never fired; fired: %v", phase, f.keys())
	return phaseFire{}
}

func (f *flight) engines(on bool) *flight {
	f.fd.Engines[0].Running, f.fd.Engines[1].Running = on, on
	return f
}

func (f *flight) at(alt, agl, vs, gs float64) *flight {
	f.fd.Position.Altitude, f.fd.Position.AltitudeAGL = alt, agl
	f.fd.Attitude.VS, f.fd.Attitude.GS = vs, gs
	f.fd.Sensors.OnGround = agl <= 0
	return f
}

// climbTo flies from where the aircraft is to alt at vs, a reading a second.
func (f *flight) climbTo(alt, vs float64) *flight {
	for f.fd.Position.Altitude != alt {
		next := f.fd.Position.Altitude + vs/60
		if (vs > 0 && next > alt) || (vs < 0 && next < alt) {
			next = alt
		}
		f.at(next, next, vs, 250)
		f.hold(0)
	}
	return f
}

// departure takes a flight from the gate to the runway without GSX.
func departure(f *flight) *flight {
	f.hold(time.Minute)                                   // boarding at the gate
	f.fd.Lights.Beacon = true                             // doors closed
	f.hold(time.Second).at(30, 0, 0, 2).hold(time.Minute) // pushed back by a tug
	f.engines(true).at(30, 0, 0, 12).hold(time.Minute)    // taxi
	return f
}

// A whole flight without GSX, in the order the cabin hears it.
func TestPhasesOfAFlightWithoutGSX(t *testing.T) {
	f := newFlight(t, phaseConfig{
		plannedBlock: 90 * time.Minute,
		cruiseAt:     []int{50},
		meals:        []mealService{{N: 1, Timing: "after_cruise", Minutes: 10}},
	})
	departure(f)
	f.at(100, 70, 2000, 160).hold(70 * time.Second) // lift-off, low
	f.climbTo(36_000, 2000)
	f.at(36_000, 36_000, 0, 450).hold(50 * time.Minute)
	f.climbTo(9_000, -1500)
	f.at(1_500, 1_500, -700, 140)
	f.fd.Controls.GearDown = true
	f.hold(0)
	f.at(20, 10, -150, 135).hold(0) // the flare
	f.at(0, 0, -150, 130)
	f.fd.Attitude.GForce = 1.1
	f.hold(5 * time.Second)
	f.at(0, 0, 0, 10).hold(time.Minute)
	f.engines(false).at(0, 0, 0, 0).hold(time.Second)

	assert.Equal(t, []string{
		domain.PhaseBoarding, domain.PhaseBoardingComplete, domain.PhasePushback, domain.PhaseTaxi,
		domain.PhaseClimbOut, domain.PhasePassing10000, domain.PhaseCruise,
		"meal_service:1", "cruise@50",
		domain.PhaseDescend, domain.PhaseDescending10000, domain.PhaseFinalApproach,
		domain.PhaseAfterLanding, domain.PhaseGateArrival,
	}, f.keys(), "the cabin hears the flight out of order")
	assert.Equal(t, domain.ReasonFlightStarted, f.fires[0].reason.Code)
	assert.Equal(t, domain.ReasonBeaconOn, f.last(domain.PhaseBoardingComplete).reason.Code)
	assert.Equal(t, domain.ReasonMovingEnginesOff, f.last(domain.PhasePushback).reason.Code)
	assert.Equal(t, domain.LandingSmooth, f.last(domain.PhaseAfterLanding).landing)
}

// Boarding is GSX's boarding being active, not merely requested; it ends
// when GSX says it has finished, and GSX's pushback and deboarding are the
// pushback and the gate arrival.
func TestPhasesFollowGSX(t *testing.T) {
	f := newFlight(t, phaseConfig{})
	f.gsx = domain.GSXState{Connected: true}
	f.hold(time.Minute)
	assert.Empty(t, f.keys(), "with GSX running, boarding waits for GSX")

	f.gsx.Boarding.State = domain.GSXRequested
	f.hold(time.Minute)
	assert.Empty(t, f.keys(), "a requested boarding has not started")

	f.gsx.Boarding.State = domain.GSXActive
	f.hold(time.Second)
	require.Equal(t, []string{domain.PhaseBoarding}, f.keys())
	assert.Equal(t, domain.ReasonGSXBoarding, f.fires[0].reason.Code)

	f.gsx.Boarding = domain.GSXService{State: domain.GSXCallable, Completed: true}
	f.hold(time.Second)
	assert.Equal(t, domain.ReasonGSXBoardingDone, f.last(domain.PhaseBoardingComplete).reason.Code)

	f.gsx.Departure.State = domain.GSXActive
	f.hold(time.Second)
	assert.Equal(t, domain.ReasonGSXPushback, f.last(domain.PhasePushback).reason.Code)

	f.engines(true).at(30, 0, 0, 12).hold(time.Second)
	f.at(500, 400, 1500, 160).hold(2 * time.Minute)
	f.at(0, 0, -200, 120).hold(time.Second)
	f.at(0, 0, 0, 10).hold(time.Second)
	f.gsx.Deboarding.State = domain.GSXActive
	f.hold(time.Second)
	assert.Equal(t, domain.ReasonGSXDeboarding, f.last(domain.PhaseGateArrival).reason.Code)
}

// Auto-start starts a flight once the beacon is on and an engine runs: the
// doors are closed, so boarding is over before it is announced.
func TestAFlightStartedReadyToGoSkipsBoarding(t *testing.T) {
	f := newFlight(t, phaseConfig{})
	f.fd.Lights.Beacon = true
	f.engines(true).hold(time.Second)
	assert.Equal(t, []string{domain.PhaseBoardingComplete}, f.keys())
	assert.Equal(t, domain.ReasonStartedReady, f.fires[0].reason.Code)
	assert.True(t, f.d.skipped[domain.PhaseBoarding])
}

// Taxiing out without a pushback still closes boarding first.
func TestTaxiWithoutPushbackClosesBoardingFirst(t *testing.T) {
	f := newFlight(t, phaseConfig{})
	f.hold(time.Second)
	f.engines(true).at(30, 0, 0, 10).hold(time.Second)
	assert.Equal(t, []string{domain.PhaseBoarding, domain.PhaseBoardingComplete, domain.PhaseTaxi}, f.keys())
	assert.True(t, f.d.skipped[domain.PhasePushback])
}

// The touchdown's rate is the sink just before the wheels touch, its load
// the largest just after; together they pick the landing announcement.
func TestTouchdownClassification(t *testing.T) {
	cases := []struct {
		name  string
		vs, g float64
		want  string
	}{
		{"butter", -120, 1.08, domain.LandingSmooth},
		{"firm but fine", -260, 1.25, domain.LandingNormal},
		{"hard by rate", -480, 1.3, domain.LandingHard},
		{"hard by load", -180, 1.6, domain.LandingHard},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFlight(t, phaseConfig{landing: domain.DefaultLandingThresholds})
			f.at(800, 600, 1200, 160).hold(2 * time.Minute)
			f.at(30, 20, c.vs, 140)
			f.fd.Attitude.GForce = 1.0
			f.hold(0) // a second later:
			// On the ground the simulator already reads the sink as nearly
			// zero; the load spikes a moment later.
			f.at(0, 0, -20, 135)
			f.fd.Attitude.GForce = c.g
			f.hold(0)
			f.fd.Attitude.GForce = 1.0
			f.at(0, 0, 0, 10).hold(time.Second)

			got := f.last(domain.PhaseAfterLanding)
			assert.Equal(t, c.want, got.landing)
			assert.Equal(t, int(c.vs), got.reason.VS)
			assert.InDelta(t, c.g, got.reason.G, 0.001)
		})
	}
}

// The announcement waits for the rollout to end, and a bounce is one landing.
func TestAfterLandingWaitsForTheRolloutAndIgnoresABounce(t *testing.T) {
	f := newFlight(t, phaseConfig{})
	f.at(800, 600, 1200, 160).hold(2 * time.Minute)
	f.at(20, 10, -350, 140).hold(0)
	f.at(0, 0, -50, 135).hold(0)
	f.at(5, 3, 100, 130).hold(time.Second) // bounce
	f.at(0, 0, -60, 125).hold(10 * time.Second)
	assert.NotContains(t, f.keys(), domain.PhaseAfterLanding, "still rolling out at 125 kt")

	f.at(0, 0, 0, 12).hold(time.Second)
	got := f.last(domain.PhaseAfterLanding)
	assert.Equal(t, -350, got.reason.VS, "the first contact is the landing")
	assert.Equal(t, 1, strings.Count(strings.Join(f.keys(), ","), domain.PhaseAfterLanding))
}

// The seatbelt sign is announced in flight only, after the climb-out, on a
// change of a sign the aircraft reports, and not twice within the cooldown.
func TestSeatbeltAnnouncements(t *testing.T) {
	f := newFlight(t, phaseConfig{})
	f.fd.Cabin = domain.CabinData{SeatbeltSign: true, SeatbeltKnown: true}
	f.hold(time.Second)
	f.fd.Cabin.SeatbeltSign = false
	f.hold(time.Second)
	f.fd.Cabin.SeatbeltSign = true
	f.at(400, 300, 2000, 150).hold(10 * time.Second)
	f.fd.Cabin.SeatbeltSign = false
	f.hold(time.Second)
	assert.NotContains(t, f.keys(), domain.PhaseSeatbeltOff, "not before the climb-out")

	f.fd.Cabin.SeatbeltSign = true
	f.at(5_000, 4_000, 2000, 250).hold(time.Minute)
	require.Contains(t, f.keys(), domain.PhaseClimbOut)
	f.fd.Cabin.SeatbeltSign = false
	f.hold(time.Second)
	off := f.last(domain.PhaseSeatbeltOff)
	assert.Equal(t, 5_000, off.reason.Altitude)

	f.fd.Cabin.SeatbeltSign = true
	f.hold(time.Second)
	f.fd.Cabin.SeatbeltSign = false
	f.hold(time.Second)
	assert.Equal(t, 1, strings.Count(strings.Join(f.keys(), ","), domain.PhaseSeatbeltOff),
		"off again within the two-minute cooldown")
	assert.Equal(t, 1, strings.Count(strings.Join(f.keys(), ","), domain.PhaseSeatbeltOn))

	f.fd.Cabin.SeatbeltSign = true
	f.hold(3 * time.Minute)
	f.fd.Cabin.SeatbeltSign = false
	f.hold(time.Second)
	assert.Equal(t, 2, strings.Count(strings.Join(f.keys(), ","), domain.PhaseSeatbeltOff))
}

func TestAnUnreadSeatbeltSignIsNeverAnnounced(t *testing.T) {
	f := newFlight(t, phaseConfig{})
	f.at(5_000, 4_000, 2000, 250).hold(2 * time.Minute)
	f.fd.Cabin.SeatbeltSign = true // nobody set SeatbeltKnown
	f.hold(time.Second)
	f.fd.Cabin.SeatbeltSign = false
	f.hold(time.Second)
	assert.NotContains(t, f.keys(), domain.PhaseSeatbeltOn)
	assert.NotContains(t, f.keys(), domain.PhaseSeatbeltOff)
}

// A cruise line at 75 % of a block time the flight overran is not played in
// the descent; a meal before the descent is dropped once the descent begins.
func TestCruiseLinesAndMealsGiveWayToTheDescent(t *testing.T) {
	f := newFlight(t, phaseConfig{
		plannedBlock: 60 * time.Minute,
		cruiseAt:     []int{75},
		meals:        []mealService{{N: 2, Timing: "before_descent", Minutes: 5}},
	})
	f.at(500, 400, 1500, 160).hold(time.Minute)
	f.climbTo(24_000, 2500)
	f.at(24_000, 24_000, 0, 400).hold(3 * time.Minute)
	require.Contains(t, f.keys(), domain.PhaseCruise)
	f.climbTo(12_000, -2000) // descends early, at 0:16
	f.at(12_000, 12_000, -1000, 300).hold(40 * time.Minute)

	assert.Contains(t, f.keys(), domain.PhaseDescend)
	assert.NotContains(t, f.keys(), "cruise@75")
	assert.NotContains(t, f.keys(), "meal_service:2")
	assert.True(t, f.d.skipped["cruise@75"])
	assert.True(t, f.d.skipped["meal_service:2"])
}

// The meal before the descent is timed from the planned block, half an hour
// before its end, less the service's minutes.
func TestMealBeforeDescentIsTimedFromThePlannedBlock(t *testing.T) {
	f := newFlight(t, phaseConfig{
		plannedBlock: 120 * time.Minute,
		meals:        []mealService{{N: 2, Timing: "before_descent", Minutes: 20}},
	})
	f.at(500, 400, 1500, 160).hold(time.Minute)
	f.climbTo(30_000, 2500)
	f.at(30_000, 30_000, 0, 450)
	for !f.d.has("meal_service:2") && f.now.Before(flightStart.Add(3*time.Hour)) {
		f.hold(0)
	}
	fired := f.d.fired["meal_service:2"]
	assert.Equal(t, flightStart.Add(70*time.Minute), fired, "120 min block − 30 min to top of descent − 20 min")
}

// A level-off on the climb is not the cruise: cruise needs two minutes level.
func TestCruiseNeedsTwoMinutesLevel(t *testing.T) {
	f := newFlight(t, phaseConfig{})
	f.at(500, 400, 1500, 160).hold(time.Minute)
	f.climbTo(14_000, 2500)
	f.at(14_000, 14_000, 0, 300).hold(90 * time.Second)
	assert.NotContains(t, f.keys(), domain.PhaseCruise)
	f.climbTo(34_000, 2000)
	f.at(34_000, 34_000, 50, 450).hold(cruiseHold)
	assert.Equal(t, 34_000, f.last(domain.PhaseCruise).reason.Altitude)
}
