package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	// windowStateDebounce is how long a burst of move and resize events is
	// left to settle before the window is read and the placement written.
	// Dragging fires dozens of events a second; reading the window is a round
	// trip to the UI thread, so one read per burst keeps the drag smooth, and
	// reading after the burst sees a settled window rather than one mid-way
	// through a maximise or restore.
	windowStateDebounce = 250 * time.Millisecond

	// windowRevealTimeout bounds how long a window that is waiting to be
	// shown in its saved mode stays hidden if the webview never reports
	// ready. It is longer than a cold WebView2 start on a slow disk and short
	// enough that a broken frontend does not leave the pilot with no window.
	windowRevealTimeout = 5 * time.Second
)

// windowProbe is the part of a window the manager reads. It is satisfied by
// *application.WebviewWindow and replaced by a fake in tests.
type windowProbe interface {
	IsMaximised() bool
	IsFullscreen() bool
	IsMinimised() bool
	Size() (int, int)
	Position() (int, int)
}

// windowRevealer is a probe that can also be put into its saved mode and
// shown.
type windowRevealer interface {
	windowProbe
	Maximise() application.Window
	Fullscreen() application.Window
	Show() application.Window
}

// windowStateManager remembers where the pilot left the window and puts it
// back on the next launch.
//
// The placement lives in its own file rather than in settings.json: the
// webview rewrites the whole settings object whenever any setting changes,
// and would overwrite coordinates it has never seen.
type windowStateManager struct {
	path     string
	debounce time.Duration
	// startTimer arms a one-shot timer and returns the function that stops
	// it. Tests replace it so no test waits on the clock.
	startTimer func(d time.Duration, f func()) (stop func() bool)

	// mu guards state, armed and stopTimer. It is never held across a call
	// into the window, which round-trips to the UI thread.
	mu    sync.Mutex
	state domain.WindowState
	// armed is false while a window waiting to be shown in its saved mode is
	// still in its normal, hidden state. Observing it then would overwrite
	// the saved mode with "normal".
	armed     bool
	stopTimer func() bool

	// fileMu serialises writes to path.
	fileMu sync.Mutex
}

func newWindowStateManager(path string) *windowStateManager {
	if path == "" {
		configDir, _ := os.UserConfigDir()
		path = filepath.Join(configDir, "airspace-acars", "window_state.json")
	}
	m := &windowStateManager{
		path:     path,
		debounce: windowStateDebounce,
		startTimer: func(d time.Duration, f func()) func() bool {
			return time.AfterFunc(d, f).Stop
		},
		state: loadWindowState(path),
	}
	m.armed = !m.startsInDisplayMode()
	return m
}

// loadWindowState reads the saved placement, falling back to the default when
// there is none or it cannot be trusted.
func loadWindowState(path string) domain.WindowState {
	state := domain.DefaultWindowState()
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("window state unreadable, using defaults", "window.error", bareErr(err))
		}
		return state
	}
	if err := json.Unmarshal(data, &state); err != nil {
		slog.Warn("window state corrupt, using defaults", "window.error", err)
		return domain.DefaultWindowState()
	}
	state.Sanitize()
	return state
}

func (m *windowStateManager) getState() domain.WindowState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// startsInDisplayMode reports whether the saved state is maximised or
// fullscreen.
func (m *windowStateManager) startsInDisplayMode() bool {
	state := m.getState()
	return state.Maximized || state.Fullscreen
}

// applyToOptions sets the window's size and position from the saved state,
// adjusted so it fits the displays that are connected now.
//
// The window is never asked to start maximised or fullscreen. Wails applies
// StartState before the position and then moves the already-maximised window
// to (X, Y), which leaves a display-sized window hanging off the screen. A
// window that was closed maximised is instead created hidden at its normal
// bounds, so that leaving the mode restores them, and put into the mode by
// bind once the webview is up.
func (m *windowStateManager) applyToOptions(opts *application.WebviewWindowOptions, screens []*application.Screen) {
	state := m.getState().FitTo(screenBounds(screens))

	opts.Width = state.Width
	opts.Height = state.Height
	if state.HasPosition {
		opts.X = state.X
		opts.Y = state.Y
		opts.InitialPosition = application.WindowXY
	} else {
		opts.InitialPosition = application.WindowCentered
	}
	opts.StartState = application.WindowStateNormal
	opts.Hidden = state.Maximized || state.Fullscreen
}

// screenBounds converts Wails' displays to the domain's, preferring the work
// area, which excludes the taskbar.
func screenBounds(screens []*application.Screen) []domain.ScreenBounds {
	var out []domain.ScreenBounds
	for _, s := range screens {
		if s == nil {
			continue
		}
		area := s.WorkArea
		if area.Width == 0 || area.Height == 0 {
			area = s.Bounds
		}
		out = append(out, domain.ScreenBounds{
			X: area.X, Y: area.Y, Width: area.Width, Height: area.Height,
			Primary: s.IsPrimary,
		})
	}
	return out
}

// bind starts following the window: it records changes to its placement and,
// if it was saved maximised or fullscreen, puts it back in that mode.
func (m *windowStateManager) bind(w *application.WebviewWindow) {
	changed := func(*application.WindowEvent) { m.scheduleObserve(w) }
	for _, ev := range []events.WindowEventType{
		events.Common.WindowDidResize,
		events.Common.WindowDidMove,
		events.Common.WindowMaximise,
		events.Common.WindowUnMaximise,
		events.Common.WindowFullscreen,
		events.Common.WindowUnFullscreen,
	} {
		w.OnWindowEvent(ev, changed)
	}

	if !m.startsInDisplayMode() {
		return
	}
	var once sync.Once
	reveal := func() { once.Do(func() { m.reveal(w) }) }
	// The runtime-ready event also fires on every reload in development;
	// once makes only the first count.
	w.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) { reveal() })
	time.AfterFunc(windowRevealTimeout, func() {
		defer observability.Recover()
		reveal()
	})
}

// reveal puts a window that was saved maximised or fullscreen back in that
// mode, shows it, and starts recording its placement.
func (m *windowStateManager) reveal(w windowRevealer) {
	state := m.getState()
	switch {
	case state.Fullscreen:
		w.Fullscreen()
	case state.Maximized:
		w.Maximise()
	}
	w.Show()

	m.mu.Lock()
	m.armed = true
	m.mu.Unlock()
}

// scheduleObserve arranges for the window to be read once the current burst
// of events has settled.
func (m *windowStateManager) scheduleObserve(w windowProbe) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopTimer != nil {
		m.stopTimer()
	}
	m.stopTimer = m.startTimer(m.debounce, func() {
		defer observability.Recover()
		m.observeAndSave(w)
	})
}

// flush reads the window and writes its placement now, for use just before
// the process exits. It must not be called from the UI thread, which it waits
// on to read the window.
func (m *windowStateManager) flush(w windowProbe) {
	m.mu.Lock()
	if m.stopTimer != nil {
		m.stopTimer()
		m.stopTimer = nil
	}
	m.mu.Unlock()
	m.observeAndSave(w)
}

// observeAndSave reads the window, folds it into the state and writes the
// state when it changed.
func (m *windowStateManager) observeAndSave(w windowProbe) {
	obs, stable := observeWindow(w)
	if !stable {
		// Caught between modes, say halfway through a restore, where the
		// size is the maximised one but the mode already says normal. Look
		// again once it has finished.
		m.scheduleObserve(w)
		return
	}

	m.mu.Lock()
	if !m.armed {
		m.mu.Unlock()
		return
	}
	next := m.state.Observe(obs)
	changed := next != m.state
	m.state = next
	m.mu.Unlock()

	if changed {
		// A failure is already logged; the pilot loses only where the window
		// opens next time.
		_ = m.writeToDisk(next)
	}
}

// observeWindow reads the window's mode, size and position. The mode is read
// before and after the geometry and the reading is reported unstable if it
// changed, so geometry from one mode is never paired with the label of
// another.
func observeWindow(w windowProbe) (domain.WindowObservation, bool) {
	read := func() domain.WindowObservation {
		return domain.WindowObservation{
			Maximized:  w.IsMaximised(),
			Fullscreen: w.IsFullscreen(),
			Minimized:  w.IsMinimised(),
		}
	}
	before := read()
	width, height := w.Size()
	x, y := w.Position()
	after := read()

	after.Width, after.Height, after.X, after.Y = width, height, x, y
	before.Width, before.Height, before.X, before.Y = width, height, x, y
	return after, before == after
}

// writeToDisk replaces the state file. It writes beside it and renames, so a
// crash mid-write cannot leave a truncated file for the next launch.
func (m *windowStateManager) writeToDisk(state domain.WindowState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal window state: %w", err)
	}

	m.fileMu.Lock()
	defer m.fileMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		slog.Warn("window state not saved", "window.error", bareErr(err))
		return fmt.Errorf("create config dir: %w", err)
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		slog.Warn("window state not saved", "window.error", bareErr(err))
		return fmt.Errorf("write window state: %w", err)
	}
	if err := os.Rename(tmp, m.path); err != nil {
		slog.Warn("window state not saved", "window.error", bareErr(err))
		return fmt.Errorf("replace window state: %w", err)
	}
	return nil
}

// bareErr strips the file path from an operating-system error, which would
// otherwise carry the pilot's profile directory into the log.
func bareErr(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return linkErr.Err
	}
	return err
}
