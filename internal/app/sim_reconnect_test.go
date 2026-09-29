package app

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/internal/profiles"
)

// fakeSim is a simulator a test can script: it answers or stays silent, and
// counts what was done to it. It is profile-aware, so a test can see whether
// the aircraft profile was handed to it.
type fakeSim struct {
	mu          sync.Mutex
	answering   bool
	stale       bool
	heard       time.Time
	connects    int
	disconnects int
	profiles    int
}

func (f *fakeSim) Name() string { return "X-Plane" }

func (f *fakeSim) Connect() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connects++
	return nil
}

func (f *fakeSim) Disconnect() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disconnects++
	return nil
}

func (f *fakeSim) GetFlightData() (*domain.FlightData, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.answering {
		return nil, errors.New("no data from simulator")
	}
	f.heard = time.Now()
	return &domain.FlightData{}, nil
}

// LastReceived is when the fake last answered, or a minute ago once it has
// gone stale: the way SimConnect goes on handing out its last reading after
// the simulator stops answering.
func (f *fakeSim) LastReceived() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.stale:
		return time.Now().Add(-time.Minute)
	case f.answering:
		f.heard = time.Now()
	}
	return f.heard
}

func (f *fakeSim) ApplyProfile(*profiles.Plan) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.profiles++
	return nil
}

func (f *fakeSim) RawIdentity() profiles.Context {
	return profiles.Context{AircraftName: "Test Aircraft", Simulator: profiles.SimXPlane, EngineCount: 2}
}

func (f *fakeSim) SupportedSources() []profiles.SourceKind {
	return []profiles.SourceKind{profiles.SourceDataRef}
}

func (f *fakeSim) set(change func(*fakeSim)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change(f)
}

func (f *fakeSim) counts() (connects, disconnects, applied int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connects, f.disconnects, f.profiles
}

// stateUI records the connection states the frontend is sent.
type stateUI struct {
	mu     sync.Mutex
	states []string
}

func (u *stateUI) EmitEvent(name string, data interface{}) {
	if name != "connection-state" {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.states = append(u.states, data.(string))
}

func (u *stateUI) sent() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.states...)
}

// newSimApp returns an App with no SimConnect, the way the ACARS runs on
// anything but Windows, whose X-Plane adapter is each of sims in turn: one per
// connection attempt, the last one repeating.
func newSimApp(t *testing.T, sims ...*fakeSim) (*App, *stateUI) {
	t.Helper()
	var mu sync.Mutex
	next := 0
	ui := &stateUI{}
	a := &App{
		UI:                   ui,
		DB:                   noopStorage{},
		NewSimConnectAdapter: func() domain.SimConnector { return nil },
		NewXPlaneAdapter: func(string, int) domain.SimConnector {
			mu.Lock()
			defer mu.Unlock()
			sim := sims[next]
			if next < len(sims)-1 {
				next++
			}
			return sim
		},
		state: "idle",
	}
	t.Cleanup(a.DisconnectSim)
	return a, ui
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The reported symptom: the ACARS loses the simulator in a stutter and does
// not come back. An attempt that heard nothing used to end in DisconnectSim,
// which is the pilot's disconnect button: it told the auto-connect loop to
// stand down, and the loop skipped every turn after that. The first attempt
// that missed was the last one made.
func TestAutoConnectKeepsTryingAfterAMiss(t *testing.T) {
	silent := &fakeSim{}
	recovered := &fakeSim{answering: true}
	a, _ := newSimApp(t, silent, recovered)

	a.autoConnect()
	if a.IsConnected() {
		t.Fatal("connected to a simulator that never answered")
	}
	if _, disconnects, _ := silent.counts(); disconnects != 1 {
		t.Errorf("the silent adapter was closed %d times, want 1", disconnects)
	}

	a.autoConnect()
	if !a.IsConnected() {
		t.Fatal("the auto-connect loop stood down after one missed attempt")
	}
}

// A simulator that stalls mid-flight keeps its connection open and sends
// nothing. The stale connection is dropped, and the next turn of the loop —
// a second later — connects again. The aircraft profile goes to the new
// adapter too: it starts without one, and the aircraft is the same as before,
// so a profile left over from the old connection read as "nothing changed" and
// was never handed over.
func TestStaleConnectionIsDroppedAndReconnected(t *testing.T) {
	stalled := &fakeSim{answering: true}
	fresh := &fakeSim{answering: true}
	a, ui := newSimApp(t, stalled, fresh)
	a.profileRegistry = profiles.NewRegistry()

	a.autoConnect()
	if !a.IsConnected() {
		t.Fatal("did not connect to a simulator that answered")
	}

	stalled.set(func(f *fakeSim) { f.stale = true })
	waitFor(t, "the stale connection is dropped", func() bool {
		_, disconnects, _ := stalled.counts()
		return disconnects == 1
	})

	a.autoConnect()
	if !a.IsConnected() {
		t.Fatal("did not reconnect after dropping a stale connection")
	}
	waitFor(t, "the new adapter is given the aircraft profile", func() bool {
		_, _, applied := fresh.counts()
		return applied > 0
	})
	if got, want := ui.sent(), []string{"X-Plane", "", "X-Plane"}; !reflect.DeepEqual(got, want) {
		t.Errorf("connection states sent = %q, want %q", got, want)
	}
}

// X-Plane stops sending in a long stutter, and the connection is marked lost.
// Its adapter is still subscribed, though, and picks the simulator back up the
// moment it answers, so the loop leaves it to do that for as long as a live
// connection would be given — replacing it would send a simulator that is not
// reading them a new set of subscriptions. Quiet for longer, it is replaced.
func TestAQuietConnectionIsGivenTimeToComeBack(t *testing.T) {
	sim := &fakeSim{answering: true}
	replacement := &fakeSim{answering: true}
	a, _ := newSimApp(t, sim, replacement)

	a.autoConnect()
	if !a.IsConnected() {
		t.Fatal("did not connect to a simulator that answered")
	}

	sim.set(func(f *fakeSim) { f.answering = false })
	waitFor(t, "the connection is marked lost", func() bool { return !a.IsConnected() })

	a.autoConnect()
	if connects, disconnects, _ := sim.counts(); connects != 1 || disconnects != 0 {
		t.Fatalf("a connection quiet for a moment was replaced: opened %d and closed %d times", connects, disconnects)
	}

	sim.set(func(f *fakeSim) { f.answering = true })
	waitFor(t, "the same connection picks the simulator back up", a.IsConnected)

	sim.set(func(f *fakeSim) { f.answering, f.stale = false, true })
	waitFor(t, "the connection is marked lost again", func() bool { return !a.IsConnected() })

	a.autoConnect()
	if _, disconnects, _ := sim.counts(); disconnects != 1 {
		t.Errorf("a connection quiet for too long was closed %d times, want 1", disconnects)
	}
	if connects, _, _ := replacement.counts(); connects != 1 || !a.IsConnected() {
		t.Error("a connection quiet for too long was not replaced")
	}
}

// At one attempt a second, the loop is nearly always mid-attempt while nothing
// is connected, so a pilot pressing Connect lands on top of it. The loop must
// not close the pilot's adapter and open one of its own while the pilot's is
// still waiting for the simulator.
func TestAutoConnectLeavesAPilotsConnectAlone(t *testing.T) {
	sim := &fakeSim{}
	a, _ := newSimApp(t, sim)

	done := make(chan error, 1)
	go func() {
		_, err := a.ConnectSim("xplane")
		done <- err
	}()
	waitFor(t, "the pilot's attempt opens its adapter", func() bool {
		connects, _, _ := sim.counts()
		return connects == 1
	})

	a.autoConnect()
	sim.set(func(f *fakeSim) { f.answering = true })

	if err := <-done; err != nil {
		t.Fatalf("the pilot's connect failed: %v", err)
	}
	if connects, disconnects, _ := sim.counts(); connects != 1 || disconnects != 0 {
		t.Errorf("adapter opened %d and closed %d times, want once and never", connects, disconnects)
	}
}

// A pilot who disconnects by hand is not reconnected behind their back.
func TestAutoConnectStandsDownAfterThePilotDisconnects(t *testing.T) {
	sim := &fakeSim{answering: true}
	a, _ := newSimApp(t, sim)

	a.autoConnect()
	if !a.IsConnected() {
		t.Fatal("did not connect to a simulator that answered")
	}

	a.DisconnectSim()
	a.autoConnect()

	if a.IsConnected() {
		t.Error("reconnected after the pilot disconnected")
	}
	if connects, _, _ := sim.counts(); connects != 1 {
		t.Errorf("adapter opened %d times, want 1", connects)
	}
}
