package domain_test

import (
	"testing"

	"airspace-acars/internal/domain"
)

func TestDefaultWindowState(t *testing.T) {
	ws := domain.DefaultWindowState()
	if ws.Width != domain.DefaultWindowWidth {
		t.Fatalf("expected default width %d, got %d", domain.DefaultWindowWidth, ws.Width)
	}
	if ws.Height != domain.DefaultWindowHeight {
		t.Fatalf("expected default height %d, got %d", domain.DefaultWindowHeight, ws.Height)
	}
	if ws.Maximized {
		t.Fatal("expected default Maximized to be false")
	}
	if ws.Fullscreen {
		t.Fatal("expected default Fullscreen to be false")
	}
	if ws.HasPosition {
		t.Fatal("expected default HasPosition to be false")
	}
}

func TestWindowStateSanitize(t *testing.T) {
	cases := []struct {
		name       string
		in         domain.WindowState
		wantWidth  int
		wantHeight int
	}{
		{
			name: "valid custom bounds",
			in: domain.WindowState{
				Width:  1280,
				Height: 800,
			},
			wantWidth:  1280,
			wantHeight: 800,
		},
		{
			name: "zero bounds falls back to default",
			in: domain.WindowState{
				Width:  0,
				Height: 0,
			},
			wantWidth:  domain.DefaultWindowWidth,
			wantHeight: domain.DefaultWindowHeight,
		},
		{
			name: "sub-minimum bounds falls back to default",
			in: domain.WindowState{
				Width:  domain.MinWindowWidth - 1,
				Height: domain.MinWindowHeight - 1,
			},
			wantWidth:  domain.DefaultWindowWidth,
			wantHeight: domain.DefaultWindowHeight,
		},
		{
			name: "exact minimum bounds preserved",
			in: domain.WindowState{
				Width:  domain.MinWindowWidth,
				Height: domain.MinWindowHeight,
			},
			wantWidth:  domain.MinWindowWidth,
			wantHeight: domain.MinWindowHeight,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := tc.in
			ws.Sanitize()
			if ws.Width != tc.wantWidth {
				t.Errorf("width: want %d, got %d", tc.wantWidth, ws.Width)
			}
			if ws.Height != tc.wantHeight {
				t.Errorf("height: want %d, got %d", tc.wantHeight, ws.Height)
			}
		})
	}
}

func TestWindowStateIsVisibleOnScreens(t *testing.T) {
	primary := domain.ScreenBounds{X: 0, Y: 0, Width: 1920, Height: 1080}
	secondary := domain.ScreenBounds{X: 1920, Y: 0, Width: 2560, Height: 1440}

	t.Run("empty screen list permits placement", func(t *testing.T) {
		ws := domain.WindowState{X: 100, Y: 100, Width: 800, Height: 600}
		if !ws.IsVisibleOnScreens(nil) {
			t.Fatal("expected empty screen list to return true for headless compatibility")
		}
	})

	t.Run("fully within primary screen", func(t *testing.T) {
		ws := domain.WindowState{X: 100, Y: 100, Width: 1100, Height: 700}
		if !ws.IsVisibleOnScreens([]domain.ScreenBounds{primary}) {
			t.Fatal("expected window to be visible on primary screen")
		}
	})

	t.Run("on secondary screen", func(t *testing.T) {
		ws := domain.WindowState{X: 2000, Y: 200, Width: 1200, Height: 800}
		if !ws.IsVisibleOnScreens([]domain.ScreenBounds{primary, secondary}) {
			t.Fatal("expected window to be visible on secondary screen")
		}
	})

	t.Run("saved on disconnected secondary screen", func(t *testing.T) {
		// Secondary was unplugged, only primary remains
		ws := domain.WindowState{X: 2000, Y: 200, Width: 1200, Height: 800}
		if ws.IsVisibleOnScreens([]domain.ScreenBounds{primary}) {
			t.Fatal("expected window to be reported non-visible when secondary screen is missing")
		}
	})

	t.Run("completely off-screen negative coordinates", func(t *testing.T) {
		ws := domain.WindowState{X: -2000, Y: -2000, Width: 1100, Height: 700}
		if ws.IsVisibleOnScreens([]domain.ScreenBounds{primary}) {
			t.Fatal("expected window far in negative space to be reported non-visible")
		}
	})

	t.Run("marginal overlap below threshold is rejected", func(t *testing.T) {
		// Only 50px overlap along X
		ws := domain.WindowState{X: 1920 - 50, Y: 100, Width: 1100, Height: 700}
		if ws.IsVisibleOnScreens([]domain.ScreenBounds{primary}) {
			t.Fatal("expected window with sub-100px overlap to be rejected")
		}
	})

	t.Run("sufficient overlap is accepted", func(t *testing.T) {
		// 150px overlap along X
		ws := domain.WindowState{X: 1920 - 150, Y: 100, Width: 1100, Height: 700}
		if !ws.IsVisibleOnScreens([]domain.ScreenBounds{primary}) {
			t.Fatal("expected window with 150px overlap to be accepted")
		}
	})
}
