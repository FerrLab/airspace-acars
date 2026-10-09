package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"airspace-acars/internal/domain"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// defaultWindowSaveDebounce is how long window resize and move events are coalesced
// before persisting to disk. It balances immediate persistence against high disk I/O
// during smooth user resizing or dragging.
const defaultWindowSaveDebounce = 300 * time.Millisecond

type windowStateManager struct {
	path         string
	mu           sync.Mutex
	state        domain.WindowState
	saveTimer    *time.Timer
	saveDebounce time.Duration
}

func newWindowStateManager(path string) *windowStateManager {
	if path == "" {
		configDir, _ := os.UserConfigDir()
		path = filepath.Join(configDir, "airspace-acars", "window_state.json")
	}

	mgr := &windowStateManager{
		path:         path,
		state:        domain.DefaultWindowState(),
		saveDebounce: defaultWindowSaveDebounce,
	}

	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &mgr.state); err != nil {
			slog.Warn("corrupted window state file, resetting to defaults", "path", path, "err", err)
			mgr.state = domain.DefaultWindowState()
		} else {
			mgr.state.Sanitize()
		}
	}

	return mgr
}

func (m *windowStateManager) getState() domain.WindowState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// applyToOptions applies the saved window size, position, and start state to WebviewWindowOptions.
// If the saved coordinates do not intersect any active screen, it centers the window.
func (m *windowStateManager) applyToOptions(opts *application.WebviewWindowOptions, screens []*application.Screen) {
	m.mu.Lock()
	state := m.state
	m.mu.Unlock()

	opts.Width = state.Width
	opts.Height = state.Height

	var screenBounds []domain.ScreenBounds
	for _, s := range screens {
		if s == nil {
			continue
		}
		// Prefer WorkArea (excludes taskbar/dock), fall back to total Bounds
		area := s.WorkArea
		if area.Width == 0 || area.Height == 0 {
			area = s.Bounds
		}
		screenBounds = append(screenBounds, domain.ScreenBounds{
			X:      area.X,
			Y:      area.Y,
			Width:  area.Width,
			Height: area.Height,
		})
	}

	if state.HasPosition && state.IsVisibleOnScreens(screenBounds) {
		opts.X = state.X
		opts.Y = state.Y
		opts.InitialPosition = application.WindowXY
	} else {
		opts.InitialPosition = application.WindowCentered
	}

	if state.Maximized {
		opts.StartState = application.WindowStateMaximised
	} else if state.Fullscreen {
		opts.StartState = application.WindowStateFullscreen
	} else {
		opts.StartState = application.WindowStateNormal
	}
}

// onApplicationStarted ensures that the maximized or fullscreen display state
// is active once the window backend is fully initialized.
func (m *windowStateManager) onApplicationStarted(w *application.WebviewWindow) {
	m.mu.Lock()
	state := m.state
	m.mu.Unlock()

	if state.Maximized && !w.IsMaximised() {
		w.Maximise()
	} else if state.Fullscreen && !w.IsFullscreen() {
		w.Fullscreen()
	}
}

// bind registers window event listeners to monitor resize, movement, and maximization.
func (m *windowStateManager) bind(w *application.WebviewWindow) {
	w.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) {
		m.handleResizeOrMove(w)
	})

	w.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) {
		m.handleResizeOrMove(w)
	})

	w.OnWindowEvent(events.Common.WindowMaximise, func(*application.WindowEvent) {
		m.mu.Lock()
		m.state.Maximized = true
		m.state.Fullscreen = false
		m.scheduleSaveLocked()
		m.mu.Unlock()
	})

	w.OnWindowEvent(events.Common.WindowUnMaximise, func(*application.WindowEvent) {
		m.mu.Lock()
		m.state.Maximized = false
		m.scheduleSaveLocked()
		m.mu.Unlock()
	})

	w.OnWindowEvent(events.Common.WindowFullscreen, func(*application.WindowEvent) {
		m.mu.Lock()
		m.state.Fullscreen = true
		m.state.Maximized = false
		m.scheduleSaveLocked()
		m.mu.Unlock()
	})

	w.OnWindowEvent(events.Common.WindowUnFullscreen, func(*application.WindowEvent) {
		m.mu.Lock()
		m.state.Fullscreen = false
		m.scheduleSaveLocked()
		m.mu.Unlock()
	})

	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		m.saveSync()
	})
}

func (m *windowStateManager) handleResizeOrMove(w *application.WebviewWindow) {
	if w.IsMaximised() || w.IsFullscreen() || w.IsMinimised() {
		return
	}

	width, height := w.Size()
	x, y := w.Position()

	m.mu.Lock()
	defer m.mu.Unlock()

	if width >= domain.MinWindowWidth && height >= domain.MinWindowHeight {
		m.state.Width = width
		m.state.Height = height
	}
	m.state.X = x
	m.state.Y = y
	m.state.HasPosition = true
	m.state.Maximized = false
	m.state.Fullscreen = false
	m.scheduleSaveLocked()
}

func (m *windowStateManager) scheduleSaveLocked() {
	if m.saveTimer != nil {
		m.saveTimer.Stop()
	}
	m.saveTimer = time.AfterFunc(m.saveDebounce, func() {
		m.mu.Lock()
		state := m.state
		m.mu.Unlock()
		_ = m.writeToDisk(state)
	})
}

// saveSync cancels any pending debounce timer and writes immediately to disk.
func (m *windowStateManager) saveSync() {
	m.mu.Lock()
	if m.saveTimer != nil {
		m.saveTimer.Stop()
		m.saveTimer = nil
	}
	state := m.state
	m.mu.Unlock()

	_ = m.writeToDisk(state)
}

func (m *windowStateManager) writeToDisk(state domain.WindowState) error {
	state.Sanitize()
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("failed to create config dir for window state", "path", m.path, "err", err)
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal window state: %w", err)
	}

	if err := os.WriteFile(m.path, data, 0o644); err != nil {
		slog.Warn("failed to write window state", "path", m.path, "err", err)
		return fmt.Errorf("write window state: %w", err)
	}

	return nil
}
