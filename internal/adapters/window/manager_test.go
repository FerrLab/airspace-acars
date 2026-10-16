package window

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"airspace-acars/internal/domain"
)

// fakeWindow stands in for the live window. onSize, when set, runs once in
// the middle of a reading, to simulate the window changing mode under it.
type fakeWindow struct {
	maximised, fullscreen, minimised bool
	width, height, x, y              int
	calls                            []string
	onSize                           func()
}

func (f *fakeWindow) IsMaximised() bool  { return f.maximised }
func (f *fakeWindow) IsFullscreen() bool { return f.fullscreen }
func (f *fakeWindow) IsMinimised() bool  { return f.minimised }
func (f *fakeWindow) Position() (int, int) {
	return f.x, f.y
}
func (f *fakeWindow) Size() (int, int) {
	if f.onSize != nil {
		fn := f.onSize
		f.onSize = nil
		fn()
	}
	return f.width, f.height
}
func (f *fakeWindow) Maximise() {
	f.calls = append(f.calls, "maximise")
	f.maximised = true
}
func (f *fakeWindow) Fullscreen() {
	f.calls = append(f.calls, "fullscreen")
	f.fullscreen = true
}
func (f *fakeWindow) Show() {
	f.calls = append(f.calls, "show")
}

// fakeClock replaces the debounce timer so tests fire it by hand.
type fakeClock struct {
	mu      sync.Mutex
	pending func()
	armed   int
}

func (c *fakeClock) start(_ time.Duration, f func()) func() bool {
	c.mu.Lock()
	c.pending = f
	c.armed++
	c.mu.Unlock()
	return func() bool {
		c.mu.Lock()
		had := c.pending != nil
		c.pending = nil
		c.mu.Unlock()
		return had
	}
}

func (c *fakeClock) fire(t *testing.T) {
	t.Helper()
	c.mu.Lock()
	f := c.pending
	c.pending = nil
	c.mu.Unlock()
	if f == nil {
		t.Fatal("no debounce timer is armed; the change would never be saved")
	}
	f()
}

func (c *fakeClock) hasPending() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pending != nil
}

func newTestManager(t *testing.T, saved *domain.WindowState) (*Manager, *fakeClock) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "window_state.json")
	if saved != nil {
		data, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := New(path)
	clock := &fakeClock{}
	m.startTimer = clock.start
	return m, clock
}

func readSaved(t *testing.T, m *Manager) domain.WindowState {
	t.Helper()
	data, err := os.ReadFile(m.path)
	if err != nil {
		t.Fatalf("window state was never written: %v", err)
	}
	var s domain.WindowState
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("saved window state is not valid JSON: %v", err)
	}
	return s
}

var testScreen = domain.ScreenBounds{
	Primary: true,
	X:       0,
	Y:       0,
	Width:   1920,
	Height:  1040,
}

func TestFirstRunUsesTheDefaultCentredWindow(t *testing.T) {
	m, _ := newTestManager(t, nil)
	bounds := m.InitialBounds([]domain.ScreenBounds{testScreen})

	if bounds.Width != domain.DefaultWindowWidth || bounds.Height != domain.DefaultWindowHeight {
		t.Fatalf("opened %dx%d", bounds.Width, bounds.Height)
	}
	if bounds.HasPosition {
		t.Fatal("first run should be centred (HasPosition = false)")
	}
	if bounds.Hidden {
		t.Fatal("first run must show a normal window (Hidden = false)")
	}
}

func TestACorruptStateFileFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window_state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := New(path).State()
	if got != domain.DefaultWindowState() {
		t.Fatalf("got %+v, want the defaults", got)
	}
}

func TestASavedPlacementIsRestored(t *testing.T) {
	m, _ := newTestManager(t, &domain.WindowState{Width: 1200, Height: 800, X: 200, Y: 150, HasPosition: true})
	bounds := m.InitialBounds([]domain.ScreenBounds{testScreen})

	if !bounds.HasPosition || bounds.X != 200 || bounds.Y != 150 {
		t.Fatalf("position not restored: hasPos=%v (%d, %d)", bounds.HasPosition, bounds.X, bounds.Y)
	}
	if bounds.Width != 1200 || bounds.Height != 800 {
		t.Fatalf("size not restored: %dx%d", bounds.Width, bounds.Height)
	}
}

func TestAPlacementOnAnUnpluggedDisplayIsCentred(t *testing.T) {
	m, _ := newTestManager(t, &domain.WindowState{Width: 1200, Height: 800, X: 3000, Y: 3000, HasPosition: true})
	bounds := m.InitialBounds([]domain.ScreenBounds{testScreen})

	if bounds.HasPosition {
		t.Fatal("window on unplugged screen should be centred without position")
	}
}

// Regression: Wails applies StartState before the position, so asking it to
// start maximised at (X, Y) opened a display-sized window offset by (X, Y).
// The window must instead be created hidden and normal at its restored
// bounds, and put into the mode afterwards.
func TestAMaximisedWindowIsNeverStartedMaximisedByWails(t *testing.T) {
	for _, tc := range []struct {
		name  string
		saved domain.WindowState
	}{
		{"maximised", domain.WindowState{Width: 1200, Height: 800, X: 200, Y: 150, HasPosition: true, Maximized: true}},
		{"fullscreen", domain.WindowState{Width: 1200, Height: 800, X: 200, Y: 150, HasPosition: true, Fullscreen: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newTestManager(t, &tc.saved)
			bounds := m.InitialBounds([]domain.ScreenBounds{testScreen})

			if !bounds.Hidden {
				t.Fatal("the window must start hidden so a normal window does not flash before the mode is applied")
			}
			if bounds.X != 200 || bounds.Y != 150 || bounds.Width != 1200 || bounds.Height != 800 {
				t.Fatalf("the restored bounds are lost: (%d, %d) %dx%d", bounds.X, bounds.Y, bounds.Width, bounds.Height)
			}
		})
	}
}

// A state file written by the old bug holds a display-sized window at an
// offset. It must not reproduce the off-screen window.
func TestAStateFileFromTheOldBugIsRepaired(t *testing.T) {
	m, _ := newTestManager(t, &domain.WindowState{Width: 1936, Height: 1056, X: 310, Y: 235, HasPosition: true})
	bounds := m.InitialBounds([]domain.ScreenBounds{testScreen})

	if bounds.X+bounds.Width > 1920 || bounds.Y+bounds.Height > 1040 || bounds.X < 0 || bounds.Y < 0 {
		t.Fatalf("window still spills off the display: (%d, %d) %dx%d", bounds.X, bounds.Y, bounds.Width, bounds.Height)
	}
}

func TestMovingTheWindowIsSavedAfterTheBurstSettles(t *testing.T) {
	m, clock := newTestManager(t, nil)
	win := &fakeWindow{width: 1280, height: 800, x: 120, y: 90}

	// A drag fires many events; only one read is armed.
	for range 5 {
		m.ScheduleObserve(win)
	}
	if _, err := os.Stat(m.path); err == nil {
		t.Fatal("state was written before the burst settled")
	}
	clock.fire(t)

	got := readSaved(t, m)
	want := domain.WindowState{Width: 1280, Height: 800, X: 120, Y: 90, HasPosition: true}
	if got != want {
		t.Fatalf("saved %+v, want %+v", got, want)
	}
	if _, err := os.Stat(m.path + ".tmp"); err == nil {
		t.Fatal("the temporary file was left behind")
	}
}

// Regression: maximising must record the mode, not the display-sized bounds.
func TestMaximisingDoesNotOverwriteTheNormalBounds(t *testing.T) {
	m, clock := newTestManager(t, &domain.WindowState{Width: 1200, Height: 800, X: 200, Y: 150, HasPosition: true})
	win := &fakeWindow{maximised: true, width: 1936, height: 1056, x: -8, y: -8}

	m.ScheduleObserve(win)
	clock.fire(t)

	got := readSaved(t, m)
	want := domain.WindowState{Width: 1200, Height: 800, X: 200, Y: 150, HasPosition: true, Maximized: true}
	if got != want {
		t.Fatalf("saved %+v, want %+v", got, want)
	}
}

func TestRestoringAMaximisedWindowIsSaved(t *testing.T) {
	m, clock := newTestManager(t, &domain.WindowState{Width: 1200, Height: 800, X: 200, Y: 150, HasPosition: true, Maximized: true})
	m.mu.Lock()
	m.armed = true
	m.mu.Unlock()
	win := &fakeWindow{width: 1000, height: 640, x: 40, y: 60}

	m.ScheduleObserve(win)
	clock.fire(t)

	got := readSaved(t, m)
	want := domain.WindowState{Width: 1000, Height: 640, X: 40, Y: 60, HasPosition: true}
	if got != want {
		t.Fatalf("saved %+v, want %+v", got, want)
	}
}

// A reading that straddles a mode change would pair the old mode with the
// new size. It is retried instead of saved.
func TestAReadingTakenMidTransitionIsRetried(t *testing.T) {
	m, clock := newTestManager(t, nil)
	win := &fakeWindow{width: 1936, height: 1056, x: -8, y: -8}
	win.onSize = func() { win.maximised = true } // mode flips between the two flag reads

	m.ScheduleObserve(win)
	clock.fire(t) // unstable: must not save, must look again
	if _, err := os.Stat(m.path); err == nil {
		t.Fatal("an unstable reading was saved")
	}
	clock.fire(t) // settled: maximised

	want := domain.DefaultWindowState()
	want.Maximized = true
	if got := readSaved(t, m); got != want {
		t.Fatalf("saved %+v, want %+v", got, want)
	}
}

// An unstable reading retry loop must be capped so it never spins indefinitely.
func TestUnstableReadingRetryBudgetIsCapped(t *testing.T) {
	m, clock := newTestManager(t, nil)
	win := &persistentlyUnstableWindow{fakeWindow: fakeWindow{width: 1936, height: 1056}}

	m.ScheduleObserve(win)

	// Initial observation fire plus maxUnstableRetries retry fires.
	for i := 0; i <= maxUnstableRetries; i++ {
		if !clock.hasPending() {
			t.Fatalf("expected observation %d to be armed", i)
		}
		clock.fire(t)
	}

	// The budget is now exhausted; no further timer should be armed.
	if clock.hasPending() {
		t.Fatal("unstable reading retry loop did not stop after maxUnstableRetries")
	}
	if _, err := os.Stat(m.path); err == nil {
		t.Fatal("an unstable reading was saved after exhausting budget")
	}

	// A new external window event resets the budget and re-arms observation.
	m.ScheduleObserve(win)
	if !clock.hasPending() {
		t.Fatal("new event did not reset budget and re-arm timer")
	}
}

type persistentlyUnstableWindow struct {
	fakeWindow
	toggle bool
}

func (w *persistentlyUnstableWindow) Size() (int, int) {
	w.toggle = !w.toggle
	w.maximised = w.toggle
	return 1200, 800
}

func TestFlushSavesImmediatelyAndCancelsTheTimer(t *testing.T) {
	m, clock := newTestManager(t, nil)
	win := &fakeWindow{width: 1280, height: 800, x: 120, y: 90}

	m.ScheduleObserve(win)
	m.Flush(win)

	if clock.hasPending() {
		t.Fatal("the debounce timer still pends after a flush; it would write a second time")
	}
	if got := readSaved(t, m); got.X != 120 || got.Width != 1280 {
		t.Fatalf("flush did not save: %+v", got)
	}
}

func TestNothingIsWrittenWhenNothingChanged(t *testing.T) {
	m, clock := newTestManager(t, &domain.WindowState{Width: 1280, Height: 800, X: 120, Y: 90, HasPosition: true})
	if err := os.Remove(m.path); err != nil {
		t.Fatal(err)
	}

	m.ScheduleObserve(&fakeWindow{width: 1280, height: 800, x: 120, y: 90})
	clock.fire(t)

	if _, err := os.Stat(m.path); err == nil {
		t.Fatal("an unchanged placement was rewritten")
	}
}

// A window waiting to be shown in its saved mode is normal and hidden. Reading
// it then would overwrite "maximised" with "normal" if the pilot quit during
// startup.
func TestTheSavedModeSurvivesAQuitBeforeTheWindowIsRevealed(t *testing.T) {
	saved := domain.WindowState{Width: 1200, Height: 800, X: 200, Y: 150, HasPosition: true, Maximized: true}
	m, clock := newTestManager(t, &saved)
	win := &fakeWindow{width: 1200, height: 800, x: 200, y: 150}

	m.ScheduleObserve(win)
	clock.fire(t)

	if got := m.State(); got != saved {
		t.Fatalf("an unrevealed window overwrote the saved state: %+v", got)
	}
}

func TestARevealPutsTheWindowInItsSavedMode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		saved domain.WindowState
		want  []string
	}{
		{"maximised", domain.WindowState{Width: 1200, Height: 800, Maximized: true}, []string{"maximise", "show"}},
		{"fullscreen", domain.WindowState{Width: 1200, Height: 800, Fullscreen: true}, []string{"fullscreen", "show"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, clock := newTestManager(t, &tc.saved)
			win := &fakeWindow{width: 1200, height: 800}

			m.Reveal(win)

			if len(win.calls) != len(tc.want) || win.calls[0] != tc.want[0] || win.calls[1] != tc.want[1] {
				t.Fatalf("calls = %v, want %v", win.calls, tc.want)
			}
			// Now armed: a later change is recorded.
			win.maximised, win.fullscreen = false, false
			m.ScheduleObserve(win)
			clock.fire(t)
			if got := readSaved(t, m); got.Maximized || got.Fullscreen {
				t.Fatalf("after reveal the pilot leaving the mode was not saved: %+v", got)
			}
		})
	}
}

// Write ordering: a lagging debounce callback with an older version must not
// overwrite a newer state that was already written by flush.
func TestWriteOrderingDropsStaleWrite(t *testing.T) {
	m, _ := newTestManager(t, nil)

	sNew := domain.WindowState{Width: 1400, Height: 900, X: 100, Y: 100, HasPosition: true}
	sOld := domain.WindowState{Width: 1200, Height: 800, X: 50, Y: 50, HasPosition: true}

	// Write newer version 2
	if err := m.writeToDisk(sNew, 2); err != nil {
		t.Fatal(err)
	}

	// Stale write with version 1 arrives late: must be ignored
	if err := m.writeToDisk(sOld, 1); err != nil {
		t.Fatal(err)
	}

	got := readSaved(t, m)
	if got != sNew {
		t.Fatalf("stale write overwrote newer state: got %+v, want %+v", got, sNew)
	}
}

func TestBareErrStripsThePathFromOSErrors(t *testing.T) {
	if got := bareErr(&os.PathError{Op: "open", Path: `C:\Users\someone\secret`, Err: os.ErrPermission}); got != os.ErrPermission {
		t.Fatalf("bareErr kept the path: %v", got)
	}
	if got := bareErr(&os.LinkError{Op: "rename", Old: "a", New: "b", Err: os.ErrExist}); got != os.ErrExist {
		t.Fatalf("bareErr kept the paths: %v", got)
	}
}
