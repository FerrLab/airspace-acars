package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"airspace-acars/internal/domain"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestWindowStateManagerLoadDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "window_state.json")

	mgr := newWindowStateManager(path)
	state := mgr.getState()

	if state.Width != domain.DefaultWindowWidth {
		t.Fatalf("expected width %d, got %d", domain.DefaultWindowWidth, state.Width)
	}
	if state.Height != domain.DefaultWindowHeight {
		t.Fatalf("expected height %d, got %d", domain.DefaultWindowHeight, state.Height)
	}
	if state.Maximized {
		t.Fatal("expected Maximized to be false")
	}
}

func TestWindowStateManagerLoadExisting(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "window_state.json")

	saved := domain.WindowState{
		Width:       1280,
		Height:      800,
		X:           250,
		Y:           150,
		Maximized:   true,
		Fullscreen:  false,
		HasPosition: true,
	}
	data, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := newWindowStateManager(path)
	state := mgr.getState()

	if state.Width != 1280 || state.Height != 800 {
		t.Fatalf("got dimensions (%d, %d), want (1280, 800)", state.Width, state.Height)
	}
	if state.X != 250 || state.Y != 150 {
		t.Fatalf("got position (%d, %d), want (250, 150)", state.X, state.Y)
	}
	if !state.Maximized {
		t.Fatal("expected Maximized to be true")
	}
	if !state.HasPosition {
		t.Fatal("expected HasPosition to be true")
	}
}

func TestWindowStateManagerCorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "window_state.json")

	if err := os.WriteFile(path, []byte("{corrupted-json"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := newWindowStateManager(path)
	state := mgr.getState()

	if state.Width != domain.DefaultWindowWidth {
		t.Fatalf("expected fallback to default width %d, got %d", domain.DefaultWindowWidth, state.Width)
	}
}

func TestWindowStateManagerSaveSync(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "window_state.json")

	mgr := newWindowStateManager(path)
	mgr.mu.Lock()
	mgr.state = domain.WindowState{
		Width:       1366,
		Height:      768,
		X:           100,
		Y:           100,
		Maximized:   false,
		Fullscreen:  false,
		HasPosition: true,
	}
	mgr.mu.Unlock()

	mgr.saveSync()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read persisted file: %v", err)
	}

	var loaded domain.WindowState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal saved json: %v", err)
	}

	if loaded.Width != 1366 || loaded.Height != 768 {
		t.Fatalf("got (%d, %d), want (1366, 768)", loaded.Width, loaded.Height)
	}
	if loaded.X != 100 || loaded.Y != 100 {
		t.Fatalf("got pos (%d, %d), want (100, 100)", loaded.X, loaded.Y)
	}
}

func TestWindowStateManagerApplyToOptions(t *testing.T) {
	primaryScreen := &application.Screen{
		WorkArea: application.Rect{X: 0, Y: 0, Width: 1920, Height: 1080},
	}

	t.Run("centered on first run", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := newWindowStateManager(filepath.Join(tmpDir, "window_state.json"))

		var opts application.WebviewWindowOptions
		mgr.applyToOptions(&opts, []*application.Screen{primaryScreen})

		if opts.Width != domain.DefaultWindowWidth || opts.Height != domain.DefaultWindowHeight {
			t.Fatalf("unexpected dimensions: %dx%d", opts.Width, opts.Height)
		}
		if opts.InitialPosition != application.WindowCentered {
			t.Fatalf("expected WindowCentered, got %v", opts.InitialPosition)
		}
		if opts.StartState != application.WindowStateNormal {
			t.Fatalf("expected WindowStateNormal, got %v", opts.StartState)
		}
	})

	t.Run("restores position when visible on screen", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := newWindowStateManager(filepath.Join(tmpDir, "window_state.json"))
		mgr.mu.Lock()
		mgr.state = domain.WindowState{
			Width:       1200,
			Height:      800,
			X:           200,
			Y:           150,
			HasPosition: true,
		}
		mgr.mu.Unlock()

		var opts application.WebviewWindowOptions
		mgr.applyToOptions(&opts, []*application.Screen{primaryScreen})

		if opts.InitialPosition != application.WindowXY {
			t.Fatalf("expected WindowXY, got %v", opts.InitialPosition)
		}
		if opts.X != 200 || opts.Y != 150 {
			t.Fatalf("unexpected pos: (%d, %d)", opts.X, opts.Y)
		}
	})

	t.Run("falls back to centered when off-screen", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := newWindowStateManager(filepath.Join(tmpDir, "window_state.json"))
		mgr.mu.Lock()
		mgr.state = domain.WindowState{
			Width:       1200,
			Height:      800,
			X:           3000,
			Y:           3000,
			HasPosition: true,
		}
		mgr.mu.Unlock()

		var opts application.WebviewWindowOptions
		mgr.applyToOptions(&opts, []*application.Screen{primaryScreen})

		if opts.InitialPosition != application.WindowCentered {
			t.Fatalf("expected WindowCentered fallback for off-screen window, got %v", opts.InitialPosition)
		}
	})

	t.Run("sets maximized start state", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := newWindowStateManager(filepath.Join(tmpDir, "window_state.json"))
		mgr.mu.Lock()
		mgr.state = domain.WindowState{
			Width:       1100,
			Height:      700,
			Maximized:   true,
			HasPosition: false,
		}
		mgr.mu.Unlock()

		var opts application.WebviewWindowOptions
		mgr.applyToOptions(&opts, []*application.Screen{primaryScreen})

		if opts.StartState != application.WindowStateMaximised {
			t.Fatalf("expected WindowStateMaximised, got %v", opts.StartState)
		}
	})

	t.Run("sets fullscreen start state", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := newWindowStateManager(filepath.Join(tmpDir, "window_state.json"))
		mgr.mu.Lock()
		mgr.state = domain.WindowState{
			Width:       1100,
			Height:      700,
			Fullscreen:  true,
			HasPosition: false,
		}
		mgr.mu.Unlock()

		var opts application.WebviewWindowOptions
		mgr.applyToOptions(&opts, []*application.Screen{primaryScreen})

		if opts.StartState != application.WindowStateFullscreen {
			t.Fatalf("expected WindowStateFullscreen, got %v", opts.StartState)
		}
	})
}

func TestWindowStateManagerDebounce(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "window_state.json")

	mgr := newWindowStateManager(path)
	mgr.saveDebounce = 20 * time.Millisecond

	mgr.mu.Lock()
	mgr.state.Width = 1400
	mgr.scheduleSaveLocked()
	mgr.mu.Unlock()

	// Immediately, disk file should not yet exist
	if _, err := os.Stat(path); err == nil {
		t.Fatal("expected file not to be written immediately due to debounce")
	}

	// Wait for debounce timer to fire
	time.Sleep(50 * time.Millisecond)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file to be written after debounce, got error: %v", err)
	}

	var state domain.WindowState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state.Width != 1400 {
		t.Fatalf("expected width 1400, got %d", state.Width)
	}
}
