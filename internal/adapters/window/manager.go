package window

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
)

const (
	// DebounceDuration is how long a burst of move and resize events is left
	// to settle before the window is read and the placement written.
	// Dragging fires dozens of events a second; reading the window is a round
	// trip to the UI thread, so one read per burst keeps the drag smooth, and
	// reading after the burst sees a settled window rather than one mid-way
	// through a maximise or restore.
	DebounceDuration = 250 * time.Millisecond

	// RevealTimeout bounds how long a window that is waiting to be shown in
	// its saved mode stays hidden if the webview never reports ready. It is
	// longer than a cold WebView2 start on a slow disk and short enough that a
	// broken frontend does not leave the pilot with no window.
	RevealTimeout = 5 * time.Second

	// maxUnstableRetries is how many times the manager will retry observing
	// an unstable window reading before giving up. Real window events reset
	// the retry budget.
	maxUnstableRetries = 5
)

// Window represents the window operations the manager needs. It is satisfied
// by a thin wrapper around *application.WebviewWindow in package main, and
// replaced by a fake in tests.
type Window interface {
	IsMaximised() bool
	IsFullscreen() bool
	IsMinimised() bool
	Size() (int, int)
	Position() (int, int)
	Maximise()
	Fullscreen()
	Show()
}

// InitialBounds holds the starting geometry and visibility calculated from
// the saved state fitted to the current displays.
type InitialBounds struct {
	Width, Height int
	X, Y          int
	HasPosition   bool
	Hidden        bool
}

// Manager remembers where the pilot left the window and puts it back on the
// next launch.
//
// The placement lives in its own file rather than in settings.json: the
// webview rewrites the whole settings object whenever any setting changes,
// and would overwrite coordinates it has never seen.
type Manager struct {
	path     string
	debounce time.Duration
	// startTimer arms a one-shot timer and returns the function that stops
	// it. Tests replace it so no test waits on the clock.
	startTimer func(d time.Duration, f func()) (stop func() bool)

	// mu guards state, armed, stopTimer, unstableRetries and version. It is
	// never held across a call into the window, which round-trips to the UI thread.
	mu              sync.Mutex
	state           domain.WindowState
	armed           bool
	stopTimer       func() bool
	unstableRetries int
	version         uint64

	// fileMu serialises writes to path and ensures an older observation does
	// not overwrite a newer state that was already flushed or saved.
	fileMu             sync.Mutex
	lastWrittenVersion uint64
}

// New creates a window state manager reading from path, or the default
// profile directory path when empty.
func New(path string) *Manager {
	if path == "" {
		configDir, _ := os.UserConfigDir()
		path = filepath.Join(configDir, "airspace-acars", "window_state.json")
	}
	m := &Manager{
		path:     path,
		debounce: DebounceDuration,
		startTimer: func(d time.Duration, f func()) func() bool {
			return time.AfterFunc(d, f).Stop
		},
		state: loadWindowState(path),
	}
	m.armed = !m.StartsInDisplayMode()
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

// State returns a snapshot of the current window state.
func (m *Manager) State() domain.WindowState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// StartsInDisplayMode reports whether the saved state is maximised or
// fullscreen.
func (m *Manager) StartsInDisplayMode() bool {
	state := m.State()
	return state.Maximized || state.Fullscreen
}

// InitialBounds calculates the window's starting size, position and hidden
// state from the saved placement adjusted to fit the current screens.
//
// The window is never asked to start maximised or fullscreen. Wails applies
// StartState before the position and then moves the already-maximised window
// to (X, Y), which leaves a display-sized window hanging off the screen. A
// window that was closed maximised is instead created hidden at its normal
// bounds, so that leaving the mode restores them, and put into the mode once
// the webview is up.
func (m *Manager) InitialBounds(screens []domain.ScreenBounds) InitialBounds {
	state := m.State().FitTo(screens)
	return InitialBounds{
		Width:       state.Width,
		Height:      state.Height,
		X:           state.X,
		Y:           state.Y,
		HasPosition: state.HasPosition,
		Hidden:      state.Maximized || state.Fullscreen,
	}
}

// Reveal puts a window that was saved maximised or fullscreen back in that
// mode, shows it, and starts recording its placement.
func (m *Manager) Reveal(w Window) {
	state := m.State()
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

// ScheduleObserve arranges for the window to be read once the current burst
// of events has settled. A real window event resets the unstable retry budget.
func (m *Manager) ScheduleObserve(w Window) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unstableRetries = 0
	m.armTimerLocked(w)
}

func (m *Manager) armTimerLocked(w Window) {
	if m.stopTimer != nil {
		m.stopTimer()
	}
	m.stopTimer = m.startTimer(m.debounce, func() {
		defer observability.Recover()
		m.observeAndSave(w)
	})
}

// Flush reads the window and writes its placement now, for use just before
// the process exits. It must not be called from the UI thread, which it waits
// on to read the window.
func (m *Manager) Flush(w Window) {
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
func (m *Manager) observeAndSave(w Window) {
	obs, stable := observeWindow(w)
	if !stable {
		// Caught between modes, say halfway through a restore, where the
		// size is the maximised one but the mode already says normal. Look
		// again once it has finished, bounded by maxUnstableRetries.
		m.mu.Lock()
		if m.unstableRetries < maxUnstableRetries {
			m.unstableRetries++
			m.armTimerLocked(w)
			m.mu.Unlock()
			return
		}
		m.mu.Unlock()
		slog.Warn("window state reading unstable after retries, discarding observation", "window.retries", maxUnstableRetries)
		return
	}

	m.mu.Lock()
	m.unstableRetries = 0
	if !m.armed {
		m.mu.Unlock()
		return
	}
	next := m.state.Observe(obs)
	if next == m.state {
		m.mu.Unlock()
		return
	}
	m.state = next
	m.version++
	ver := m.version
	m.mu.Unlock()

	// A failure is already logged; the pilot loses only where the window
	// opens next time.
	_ = m.writeToDisk(next, ver)
}

// observeWindow reads the window's mode, size and position. The mode is read
// before and after the geometry and the reading is reported unstable if it
// changed, so geometry from one mode is never paired with the label of
// another.
func observeWindow(w Window) (domain.WindowObservation, bool) {
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
// Writes carrying a version older than or equal to the last one written are
// skipped to preserve write ordering under concurrent debounce and flush.
func (m *Manager) writeToDisk(state domain.WindowState, ver uint64) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal window state: %w", err)
	}

	m.fileMu.Lock()
	defer m.fileMu.Unlock()

	if ver <= m.lastWrittenVersion {
		return nil
	}

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
	m.lastWrittenVersion = ver
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
