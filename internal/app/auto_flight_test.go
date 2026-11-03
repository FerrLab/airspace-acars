package app

import (
	"sync"
	"testing"
	"time"

	"airspace-acars/internal/domain"
)

// --- fakes -------------------------------------------------------------------

// routeAPI answers each request by method and path, 200 with no body unless
// told otherwise, and records what was asked.
type routeAPI struct {
	stubAPI
	mu      sync.Mutex
	replies map[string]string
	asked   []string
	// noTenant makes BaseURL empty, as before the pilot has picked a tenant.
	noTenant bool
}

func (r *routeAPI) BaseURL() string {
	if r.noTenant {
		return ""
	}
	return r.stubAPI.BaseURL()
}

func (r *routeAPI) DoRequest(method, path string, body interface{}) ([]byte, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := method + " " + path
	r.asked = append(r.asked, key)
	return []byte(r.replies[key]), 200, nil
}

func (r *routeAPI) setBooking(body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.replies == nil {
		r.replies = map[string]string{}
	}
	r.replies["GET "+bookingPath] = body
}

func (r *routeAPI) count(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, k := range r.asked {
		if k == key {
			n++
		}
	}
	return n
}

type sentEvent struct {
	name string
	data interface{}
}

// eventLog records the events the frontend is sent, and lets a test wait for
// one without sleeping.
type eventLog struct {
	mu     sync.Mutex
	events []sentEvent
	notify chan string
}

func newEventLog() *eventLog { return &eventLog{notify: make(chan string, 256)} }

func (u *eventLog) EmitEvent(name string, data interface{}) {
	u.mu.Lock()
	u.events = append(u.events, sentEvent{name, data})
	u.mu.Unlock()
	select {
	case u.notify <- name:
	default:
	}
}

// sent returns the payloads of every event called name, in order.
func (u *eventLog) sent(name string) []interface{} {
	u.mu.Lock()
	defer u.mu.Unlock()
	var out []interface{}
	for _, e := range u.events {
		if e.name == name {
			out = append(out, e.data)
		}
	}
	return out
}

func (u *eventLog) waitFor(t *testing.T, name string) {
	t.Helper()
	if len(u.sent(name)) > 0 {
		return
	}
	timeout := time.After(5 * time.Second)
	for {
		select {
		case got := <-u.notify:
			if got == name {
				return
			}
		case <-timeout:
			t.Fatalf("timed out waiting for the %q event", name)
		}
	}
}

// parkedSim is a simulator whose aircraft sits wherever the test puts it.
type parkedSim struct {
	mu sync.Mutex
	fd domain.FlightData
}

func (s *parkedSim) Name() string              { return "X-Plane" }
func (s *parkedSim) Connect() error            { return nil }
func (s *parkedSim) Disconnect() error         { return nil }
func (s *parkedSim) LastReceived() time.Time   { return time.Now() }
func (s *parkedSim) set(fd *domain.FlightData) { s.mu.Lock(); s.fd = *fd; s.mu.Unlock() }
func (s *parkedSim) GetFlightData() (*domain.FlightData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fd := s.fd
	return &fd, nil
}

// --- readings ----------------------------------------------------------------

var (
	sbgr = domain.AirportInfo{ICAO: "SBGR", Latitude: -23.435556, Longitude: -46.473056}
	sbsp = domain.AirportInfo{ICAO: "SBSP", Latitude: -23.626111, Longitude: -46.656389}
	sbrj = domain.AirportInfo{ICAO: "SBRJ", Latitude: -22.910278, Longitude: -43.163056}
)

// readyAt is the aircraft at the gate, beacon on and an engine running.
func readyAt(ap domain.AirportInfo) *domain.FlightData {
	fd := &domain.FlightData{}
	fd.Position.Latitude, fd.Position.Longitude = ap.Latitude, ap.Longitude
	fd.Sensors.OnGround = true
	fd.Lights.Beacon = true
	fd.Engines[0] = domain.EngineData{Exists: true, Running: true}
	return fd
}

// shutDownAt is the aircraft parked, beacon and engines off.
func shutDownAt(ap domain.AirportInfo) *domain.FlightData {
	fd := &domain.FlightData{}
	fd.Position.Latitude, fd.Position.Longitude = ap.Latitude, ap.Longitude
	fd.Sensors.OnGround = true
	fd.Engines[0] = domain.EngineData{Exists: true}
	return fd
}

// cruising is the aircraft in the air, on its way.
func cruising() *domain.FlightData {
	fd := &domain.FlightData{}
	fd.Position.Latitude, fd.Position.Longitude = -23.2, -45.0
	fd.Position.AltitudeAGL = 35000
	fd.Lights.Beacon = true
	fd.Engines[0] = domain.EngineData{Exists: true, Running: true}
	return fd
}

// --- the decision ------------------------------------------------------------

// A start is attempted once each time the aircraft becomes ready to depart,
// not once a second for as long as it stays ready.
func TestAutoStartIsAttemptedOncePerReadiness(t *testing.T) {
	var f autoFlight
	tick := func(fd *domain.FlightData) autoAction {
		return f.step(autoTick{fd: fd, startOn: true, state: "idle"})
	}

	if got := tick(readyAt(sbgr)); got != autoStart {
		t.Fatalf("ready to depart: %v, want a start attempt", got)
	}
	if got := tick(readyAt(sbgr)); got != autoNone {
		t.Fatalf("still ready a second later: %v, want nothing; the booking would be fetched every second", got)
	}
	tick(shutDownAt(sbgr))
	if got := tick(readyAt(sbgr)); got != autoStart {
		t.Fatalf("ready again after shutting down: %v, want another attempt", got)
	}

	var off autoFlight
	if got := off.step(autoTick{fd: readyAt(sbgr), state: "idle"}); got != autoNone {
		t.Errorf("auto-start off: %v, want nothing", got)
	}
}

// A flight finishes on its own only once it has flown and is shut down at
// its arrival airport. A shutdown anywhere else is reported once and the
// flight is left open.
func TestAutoFinishOnlyAtTheArrivalAfterFlying(t *testing.T) {
	started := time.Now()
	var f autoFlight
	tick := func(fd *domain.FlightData, arrival domain.AirportInfo) autoAction {
		return f.step(autoTick{fd: fd, finishOn: true, state: "active", started: started, arrival: arrival})
	}

	if got := tick(shutDownAt(sbgr), sbgr); got != autoNone {
		t.Fatalf("shut down at the gate before taking off: %v, want nothing; a circuit would finish before it flew", got)
	}
	tick(cruising(), sbrj)
	if got := tick(shutDownAt(sbsp), sbrj); got != autoFinishSkipped {
		t.Fatalf("shut down away from the arrival: %v, want the pilot told", got)
	}
	if got := tick(shutDownAt(sbsp), sbrj); got != autoNone {
		t.Fatalf("still shut down away from the arrival: %v, want nothing more", got)
	}
	// The booking now reports the diversion to SBSP.
	if got := tick(shutDownAt(sbsp), sbsp); got != autoFinish {
		t.Fatalf("shut down at the diverted-to arrival: %v, want a finish", got)
	}
	if got := tick(shutDownAt(sbsp), sbsp); got != autoNone {
		t.Fatalf("a second later: %v, want nothing; one finish per shutdown", got)
	}
}

// Leaving the ground belongs to one flight: the next one has to fly too.
func TestAutoFinishForgetsTheLastFlightsTakeoff(t *testing.T) {
	var f autoFlight
	first := time.Now().Add(-2 * time.Hour)
	f.step(autoTick{fd: cruising(), finishOn: true, state: "active", started: first})

	next := time.Now()
	got := f.step(autoTick{fd: shutDownAt(sbrj), finishOn: true, state: "active", started: next, arrival: sbrj})
	if got != autoNone {
		t.Errorf("a new flight shut down at the gate: %v, want nothing; it has not flown", got)
	}
}

// Turning auto-finish on during the cruise still finishes this flight, and
// while it is off nothing finishes.
func TestAutoFinishSettingIsReadEachTick(t *testing.T) {
	started := time.Now()
	var f autoFlight
	f.step(autoTick{fd: cruising(), state: "active", started: started})
	if got := f.step(autoTick{fd: shutDownAt(sbrj), state: "active", started: started, arrival: sbrj}); got != autoNone {
		t.Fatalf("auto-finish off: %v, want nothing", got)
	}
	got := f.step(autoTick{fd: shutDownAt(sbrj), finishOn: true, state: "active", started: started, arrival: sbrj})
	if got != autoFinish {
		t.Errorf("auto-finish turned on after flying: %v, want a finish", got)
	}
}

// While a finish drains, and if it fails and returns to active, the same
// shutdown does not finish again every second.
func TestAFailedAutoFinishIsNotRetriedEverySecond(t *testing.T) {
	started := time.Now()
	var f autoFlight
	tick := func(state string) autoAction {
		return f.step(autoTick{fd: shutDownAt(sbrj), finishOn: true, state: state, started: started, arrival: sbrj})
	}
	f.step(autoTick{fd: cruising(), finishOn: true, state: "active", started: started})
	if got := tick("active"); got != autoFinish {
		t.Fatalf("shut down at arrival: %v, want a finish", got)
	}
	if got := tick("finishing"); got != autoNone {
		t.Fatalf("finishing: %v, want nothing", got)
	}
	if got := tick("active"); got != autoNone {
		t.Fatalf("back to active after a failed finish: %v, want nothing until the next shutdown", got)
	}
}

// --- the app -----------------------------------------------------------------

const bookingSBGRtoSBRJ = `{"id": 42, "callsign": "AZU4512",
	"departure_airport": {"icao": "SBGR", "latitude": -23.435556, "longitude": -46.473056},
	"arrival_airport": {"icao": "SBRJ", "latitude": -22.910278, "longitude": -43.163056}}`

func newAutoApp(t *testing.T, api *routeAPI) (*App, *eventLog, *parkedSim) {
	t.Helper()
	ui := newEventLog()
	sim := &parkedSim{}
	a := &App{
		Airspace:  api,
		UI:        ui,
		DB:        noopStorage{},
		connector: sim,
		simActive: true,
		state:     "idle",
		reports:   &reportTap{},
		settings:  domain.DefaultSettings(),
	}
	t.Cleanup(func() {
		if a.GetFlightState() == "active" {
			_ = a.StopFlight() // the test is over; only its goroutines matter
		}
	})
	return a, ui, sim
}

// Without a tenant the booking request would fail inside the adapter and be
// reported as a fault (AIRSPACE-ACARS-K); auto-start must not make it.
func TestAutoStartDoesNotAskForABookingWithoutATenant(t *testing.T) {
	api := &routeAPI{noTenant: true}
	a, _, sim := newAutoApp(t, api)
	sim.set(readyAt(sbsp))

	a.tryAutoStartFlight(sbsp.Latitude, sbsp.Longitude)

	if n := len(api.asked); n != 0 {
		t.Fatalf("%d requests were sent with no tenant selected: %v", n, api.asked)
	}
}

// The reported problem: a flight started on its own wherever the beacon and
// an engine came on, even at an airport the booking does not depart from.
func TestAutoStartWaitsForTheDepartureAirport(t *testing.T) {
	api := &routeAPI{}
	api.setBooking(bookingSBGRtoSBRJ)
	a, ui, sim := newAutoApp(t, api)
	sim.set(readyAt(sbsp))

	a.tryAutoStartFlight(sbsp.Latitude, sbsp.Longitude)

	if n := api.count("POST /api/v2/acars/start"); n != 0 {
		t.Fatalf("the flight was started %d times at SBSP; its booking departs from SBGR", n)
	}
	if got := a.GetFlightState(); got != "idle" {
		t.Errorf("flight state = %q, want idle", got)
	}
	skips := ui.sent("auto-flight-skipped")
	want := domain.AutoFlightSkip{Action: "start", Airport: "SBGR"}
	if len(skips) != 1 || skips[0] != want {
		t.Errorf("skip notices = %v, want one %+v", skips, want)
	}
}

func TestAutoStartStartsAtTheDepartureAirport(t *testing.T) {
	api := &routeAPI{}
	api.setBooking(bookingSBGRtoSBRJ)
	a, ui, sim := newAutoApp(t, api)
	gateLat, gateLon := domain.Destination(sbgr.Latitude, sbgr.Longitude, 45, 2500)
	fd := readyAt(sbgr)
	fd.Position.Latitude, fd.Position.Longitude = gateLat, gateLon
	sim.set(fd)

	a.tryAutoStartFlight(gateLat, gateLon)

	if n := api.count("POST /api/v2/acars/start"); n != 1 {
		t.Fatalf("POST /start sent %d times, want 1", n)
	}
	info := a.GetActiveFlightInfo()
	if info["departure"] != "SBGR" || info["arrival"] != "SBRJ" || info["callsign"] != "AZU4512" {
		t.Errorf("active flight = %v, want AZU4512 SBGR-SBRJ", info)
	}
	if got := ui.sent("auto-flight-start"); len(got) != 1 {
		t.Errorf("auto-flight-start sent %d times, want 1", len(got))
	}
	a.flightMu.Lock()
	watching := a.bookingWatch != nil
	a.flightMu.Unlock()
	if !watching {
		t.Error("the flight is not watching its booking; a diversion would never reach it")
	}
}

// A tenant whose airports carry no coordinates still auto-starts where the
// simulator's own scenery puts the aircraft on the departure airport.
func TestAutoStartTrustsTheSimulatorsScenery(t *testing.T) {
	api := &routeAPI{}
	api.setBooking(`{"id": 7, "callsign": "PTB2001",
		"departure_airport": {"icao": "SWXX"}, "arrival_airport": {"icao": "SWYY"}}`)
	a, _, sim := newAutoApp(t, api)
	field := domain.AirportInfo{ICAO: "SWXX", Latitude: -10.5, Longitude: -55.5}
	sim.set(readyAt(field))
	a.locator = layoutAt(field)

	a.tryAutoStartFlight(field.Latitude, field.Longitude)

	if n := api.count("POST /api/v2/acars/start"); n != 1 {
		t.Errorf("POST /start sent %d times, want 1; the scenery puts the aircraft on SWXX", n)
	}
}

// layoutAt is a locator holding a one-runway layout of the airport.
func layoutAt(ap domain.AirportInfo) *groundLocator {
	g := newGroundLocator(func() domain.AirportLayoutProvider { return nil })
	n1, e1 := domain.OffsetMeters(ap.Latitude, ap.Longitude, 0, -1000)
	n2, e2 := domain.OffsetMeters(ap.Latitude, ap.Longitude, 0, 1000)
	g.layout = &domain.AirportLayout{
		ICAO: ap.ICAO, RefLat: ap.Latitude, RefLon: ap.Longitude,
		Runways: []domain.Runway{{Ends: [2]domain.RunwayEnd{
			{Designator: "18", Lat: n1, Lon: e1},
			{Designator: "36", Lat: n2, Lon: e2},
		}}},
	}
	return g
}

// The whole of a diversion: shut down at the alternate before the booking
// says so, the flight stays open; the booking reports the diversion, the
// arrival moves, and the flight finishes there.
func TestAutoFinishFollowsADiversion(t *testing.T) {
	api := &routeAPI{}
	a, ui, _ := newAutoApp(t, api)
	a.state, a.bookingID, a.callsign = "active", "42", "AZU4512"
	a.departure, a.arrival = "SBGR", "SBRJ"
	a.startTime = time.Now().Add(-90 * time.Minute)

	a.checkAutoFlight(cruising())
	a.checkAutoFlight(shutDownAt(sbsp))

	if got := a.GetFlightState(); got != "active" {
		t.Fatalf("shut down at SBSP with SBRJ booked: state %q, want the flight left open", got)
	}
	if skips := ui.sent("auto-flight-skipped"); len(skips) != 1 ||
		skips[0] != (domain.AutoFlightSkip{Action: "finish", Airport: "SBRJ"}) {
		t.Fatalf("skip notices = %v, want one for SBRJ", skips)
	}

	// The pilot declares SBSP as the alternate; the next refresh reads it.
	api.setBooking(`{"id": 42, "callsign": "AZU4512",
		"departure_airport": {"icao": "SBGR"}, "arrival_airport": {"icao": "SBRJ"},
		"alternate_airport": {"icao": "SBSP", "latitude": -23.626111, "longitude": -46.656389}}`)
	w := &bookingWatcher{bookingID: "42", fetch: a.fetchBooking, onArrival: a.applyArrival,
		stopCh: make(chan struct{}), arrival: airportFromICAO("SBRJ")}
	a.bookingWatch = w
	w.refresh()

	if got := ui.sent("flight-arrival-changed"); len(got) != 1 || got[0] != "SBSP" {
		t.Fatalf("flight-arrival-changed = %v, want [SBSP]", got)
	}
	a.checkAutoFlight(shutDownAt(sbsp))

	if got := ui.sent("auto-flight-finish"); len(got) != 1 || got[0] != "SBSP" {
		t.Fatalf("auto-flight-finish = %v, want [SBSP]", got)
	}
	ui.waitFor(t, "flight-finish-complete")
	if n := api.count("POST /api/v2/acars/finish"); n != 1 {
		t.Errorf("POST /finish sent %d times, want 1", n)
	}
}
