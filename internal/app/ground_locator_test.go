package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/internal/profiles"
)

// fakeLayouts answers layout lookups with one canned reply and counts them.
type fakeLayouts struct {
	layout *domain.AirportLayout
	err    error
	calls  int
}

func (f *fakeLayouts) NearestAirportLayout(context.Context, float64, float64) (*domain.AirportLayout, error) {
	f.calls++
	return f.layout, f.err
}

// testLocator is a locator with no goroutine and a hand-driven clock: the
// test plays the goroutine's part by calling serve.
type testLocator struct {
	*groundLocator
	clock time.Time
}

func newTestLocator(p domain.AirportLayoutProvider) *testLocator {
	tl := &testLocator{clock: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)}
	tl.groundLocator = newGroundLocator(func() domain.AirportLayoutProvider { return p })
	tl.now = func() time.Time { return tl.clock }
	return tl
}

// serve runs the lookup the last Observe asked for, if it asked. It reports
// whether there was one.
func (tl *testLocator) serve() bool {
	select {
	case req := <-tl.requests:
		tl.fetch(req)
		return true
	default:
		return false
	}
}

func egll() *domain.AirportLayout {
	return &domain.AirportLayout{
		ICAO: "EGLL", RefLat: 51.4775, RefLon: -0.4614, ElevationFt: 83,
		Runways: []domain.Runway{{
			Ends: [2]domain.RunwayEnd{
				{Designator: "09L", Lat: 51.477500, Lon: -0.484961, AltFt: 79},
				{Designator: "27R", Lat: 51.477661, Lon: -0.433128, AltFt: 78},
			},
			WidthM: 50,
		}},
	}
}

// onRunway09L is an aircraft lined up on Heathrow 09L.
func onRunway09L() *domain.FlightData {
	fd := &domain.FlightData{}
	fd.Position.Latitude, fd.Position.Longitude = 51.47752, -0.4800
	fd.Attitude.HeadingTrue = 90
	fd.Sensors.OnGround = true
	return fd
}

// The first report at a new airport cannot wait for its layout, so it says
// nothing; every report after the one lookup does.
func TestTheRunwayIsReportedOnceTheLayoutHasArrived(t *testing.T) {
	sim := &fakeLayouts{layout: egll()}
	l := newTestLocator(sim)

	if fix := l.Observe(onRunway09L()); fix.Runway != nil {
		t.Fatal("named a runway before any layout was loaded")
	}
	if !l.serve() {
		t.Fatal("did not ask for the layout of the airport the aircraft is at")
	}
	for i := 0; i < 5; i++ {
		fix := l.Observe(onRunway09L())
		if fix.Runway == nil || fix.Runway.Ends[0].Designator != "09L" {
			t.Fatalf("report %d: runway = %+v, want 09L/27R", i, fix.Runway)
		}
		if l.serve() {
			t.Fatalf("report %d: looked the airport up again while still at it", i)
		}
	}
	if sim.calls != 1 {
		t.Fatalf("simulator asked for the layout %d times, want once", sim.calls)
	}
}

// An aircraft in the air is on no runway, however low it is over one.
func TestAnAirborneAircraftIsOnNoRunway(t *testing.T) {
	l := newTestLocator(&fakeLayouts{layout: egll()})
	l.Observe(onRunway09L())
	l.serve()

	fd := onRunway09L()
	fd.Sensors.OnGround = false
	fd.Position.AltitudeAGL = 10
	if fix := l.Observe(fd); fix.Runway != nil || fix.Stand != nil {
		t.Fatalf("airborne at 10 ft, reported %+v", fix)
	}
}

// On approach the arrival airport is looked up before touchdown, so the
// first report on the runway already names it; in the cruise it is not.
func TestTheArrivalAirportIsLookedUpOnApproach(t *testing.T) {
	l := newTestLocator(&fakeLayouts{layout: egll()})
	fd := onRunway09L()
	fd.Sensors.OnGround = false

	fd.Position.AltitudeAGL = 35000
	l.Observe(fd)
	if l.serve() {
		t.Fatal("looked an airport up in the cruise")
	}

	fd.Position.AltitudeAGL = 1500
	if fix := l.Observe(fd); fix.Runway != nil {
		t.Fatal("named a runway while still on approach")
	}
	if !l.serve() {
		t.Fatal("did not look the arrival airport up on approach")
	}
	if fix := l.Observe(onRunway09L()); fix.Runway == nil {
		t.Fatal("the first report after touchdown names no runway")
	}
}

// Where there is no airport, asking again on every tick only finds nothing
// faster; the lookup waits layoutRetryEvery.
func TestLookupsAreSpacedOutWhereThereIsNoAirport(t *testing.T) {
	sim := &fakeLayouts{err: domain.ErrNoAirportData}
	l := newTestLocator(sim)

	l.Observe(onRunway09L())
	l.serve()
	l.clock = l.clock.Add(layoutRetryEvery - time.Second)
	l.Observe(onRunway09L())
	if l.serve() {
		t.Fatalf("asked again after %v; the gap is %v", layoutRetryEvery-time.Second, layoutRetryEvery)
	}
	l.clock = l.clock.Add(time.Second)
	l.Observe(onRunway09L())
	if !l.serve() {
		t.Fatal("never asked again")
	}
}

// After leaving an airport its runways are not reported anywhere else.
func TestLeavingTheAirportDropsItsLayout(t *testing.T) {
	sim := &fakeLayouts{layout: egll()}
	l := newTestLocator(sim)
	l.Observe(onRunway09L())
	l.serve()

	elsewhere := onRunway09L()
	elsewhere.Position.Latitude, elsewhere.Position.Longitude = 40.6413, -73.7781
	sim.layout, sim.err = nil, domain.ErrNoAirportData
	if fix := l.Observe(elsewhere); fix.Runway != nil {
		t.Fatal("reported a Heathrow runway at JFK")
	}
	l.clock = l.clock.Add(layoutRetryEvery)
	l.Observe(elsewhere)
	if !l.serve() {
		t.Fatal("did not look the new airport up")
	}
	if l.layout != nil {
		t.Fatal("kept Heathrow's layout after finding no airport at the new position")
	}
}

// A simulator that cannot describe its airports leaves runway and stand null
// and says so once, not on every retry of a multi-hour flight.
func TestAFailingLookupWarnsOnce(t *testing.T) {
	logs := captureLogs(t)
	sim := &fakeLayouts{err: errors.New("facility request timed out")}
	l := newTestLocator(sim)

	for i := 0; i < 4; i++ {
		if fix := l.Observe(onRunway09L()); fix.Runway != nil || fix.Stand != nil {
			t.Fatalf("lookup %d failed but the report has %+v", i, fix)
		}
		l.serve()
		l.clock = l.clock.Add(layoutRetryEvery)
	}
	warns := 0
	for _, r := range logs() {
		if r["level"] == "WARN" {
			warns++
		}
	}
	if warns != 1 {
		t.Fatalf("%d warnings over 4 failed lookups, want 1", warns)
	}
}

// An adapter that cannot read layouts at all is the same as no airport.
func TestASimulatorWithoutLayoutsReportsNull(t *testing.T) {
	l := newTestLocator(nil)
	l.groundLocator.provider = func() domain.AirportLayoutProvider { return nil }
	l.Observe(onRunway09L())
	l.serve()
	if fix := l.Observe(onRunway09L()); fix.Runway != nil || fix.Stand != nil {
		t.Fatalf("reported %+v from a simulator with no layouts", fix)
	}
}

// The server reads null as "not on one", so null must be what goes out,
// in a report sent now and in one replayed from the outbox.
func TestTheReportCarriesRunwayAndStandOrNull(t *testing.T) {
	a := &App{}
	fd := onRunway09L()

	none, err := json.Marshal(a.buildPositionReport(fd, groundFix{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"runway":null`, `"stand":null`} {
		if !strings.Contains(string(none), want) {
			t.Errorf("report off any runway or stand lacks %s", want)
		}
	}

	rw := egll().Runways[0]
	st := domain.Stand{Name: "A12", Lat: 51.4712, Lon: -0.4598}
	full, err := json.Marshal(a.buildPositionReport(fd, groundFix{Runway: &rw, Stand: &st}))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Runway struct {
			Ends []struct {
				Designator string `json:"designator"`
				Latitude   struct {
					Value float64 `json:"value"`
					Unit  string  `json:"unit"`
				} `json:"latitude"`
				Altitude struct {
					Value float64 `json:"value"`
					Unit  string  `json:"unit"`
				} `json:"altitude"`
			} `json:"ends"`
		} `json:"runway"`
		Stand struct {
			Name string `json:"name"`
		} `json:"stand"`
	}
	if err := json.Unmarshal(full, &got); err != nil {
		t.Fatal(err)
	}
	ends := got.Runway.Ends
	if len(ends) != 2 || ends[0].Designator != "09L" || ends[1].Designator != "27R" {
		t.Fatalf("runway ends = %+v, want 09L and 27R", ends)
	}
	if ends[0].Latitude.Unit != "deg" || ends[0].Altitude.Unit != "ft" || ends[0].Altitude.Value != 79 {
		t.Errorf("09L end = %+v, want latitude in deg and altitude 79 ft", ends[0])
	}
	if got.Stand.Name != "A12" {
		t.Errorf("stand name = %q, want A12", got.Stand.Name)
	}
}

// The debug screen reads where the locator last placed the aircraft.
func TestStatusNamesTheAirportAndWhereTheAircraftIs(t *testing.T) {
	l := newTestLocator(&fakeLayouts{layout: egll()})
	if s := l.Status(); s.Airport != "" || !s.LoadedAt.IsZero() {
		t.Fatalf("status before any lookup = %+v, want empty", s)
	}

	l.Observe(onRunway09L())
	l.serve()
	l.Observe(onRunway09L())

	s := l.Status()
	if s.Airport != "EGLL" || s.Runways != 1 || s.Runway != "09L/27R" || s.Stand != "" {
		t.Fatalf("status = %+v, want EGLL with 1 runway, on 09L/27R, no stand", s)
	}
	if !s.LoadedAt.Equal(l.clock) {
		t.Errorf("layout loaded at %v, want %v", s.LoadedAt, l.clock)
	}

	climbing := onRunway09L()
	climbing.Sensors.OnGround = false
	climbing.Position.AltitudeAGL = 500
	l.Observe(climbing)
	if s := l.Status(); s.Runway != "" || s.Airport != "EGLL" {
		t.Fatalf("after lift-off, status = %+v; want EGLL kept and no runway", s)
	}

	var none *groundLocator
	if s := none.Status(); s.Airport != "" {
		t.Fatalf("a nil locator reported %+v", s)
	}
}

// A failed lookup stays on screen until one works. "No airport here" is an
// answer, not a failure, and shows no error.
func TestStatusShowsTheLastLookupFailureUntilOneWorks(t *testing.T) {
	captureLogs(t)
	sim := &fakeLayouts{err: errors.New("facility data for SBRF incomplete after 8s")}
	l := newTestLocator(sim)

	l.Observe(onRunway09L())
	l.serve()
	if s := l.Status(); s.LastError != "facility data for SBRF incomplete after 8s" {
		t.Fatalf("last error = %q, want the lookup's failure", s.LastError)
	}

	sim.err, sim.layout = nil, egll()
	l.clock = l.clock.Add(layoutRetryEvery)
	l.Observe(onRunway09L())
	l.serve()
	if s := l.Status(); s.LastError != "" || s.Airport != "EGLL" {
		t.Fatalf("after a successful lookup, status = %+v", s)
	}

	sim.err, sim.layout = domain.ErrNoAirportData, nil
	far := onRunway09L()
	far.Position.Latitude = 40
	l.clock = l.clock.Add(layoutRetryEvery)
	l.Observe(far)
	l.serve()
	if s := l.Status(); s.LastError != "" || s.Airport != "" || !s.LoadedAt.IsZero() {
		t.Fatalf("with no airport around, status = %+v, want empty with no error", s)
	}
}

// The locator runs for as long as a simulator is connected, flight or not,
// so the stand can be checked at the gate before departure.
func TestTheLocatorRunsWhileASimulatorIsConnected(t *testing.T) {
	sim := &fakeSim{answering: true}
	a, _ := newSimApp(t, sim)
	a.profileRegistry = profiles.NewRegistry()

	if a.currentLocator() != nil {
		t.Fatal("a locator was running before any simulator connected")
	}
	a.autoConnect()
	waitFor(t, "the locator starts with the connection", func() bool { return a.currentLocator() != nil })

	a.DisconnectSim()
	waitFor(t, "the locator stops with the connection", func() bool { return a.currentLocator() == nil })
}

// A reconnect replaces the locator. The flight loop asks for the current one
// on every sample; a stopped one would never look an airport up again.
func TestAReconnectGetsAFreshLocator(t *testing.T) {
	stalled := &fakeSim{answering: true}
	fresh := &fakeSim{answering: true}
	a, _ := newSimApp(t, stalled, fresh)
	a.profileRegistry = profiles.NewRegistry()

	a.autoConnect()
	waitFor(t, "the first connection's locator", func() bool { return a.currentLocator() != nil })
	first := a.currentLocator()

	stalled.set(func(f *fakeSim) { f.stale = true })
	waitFor(t, "the stale connection is dropped", func() bool { return a.currentLocator() == nil })
	a.autoConnect()
	waitFor(t, "the new connection's locator", func() bool {
		l := a.currentLocator()
		return l != nil && l != first
	})
}
