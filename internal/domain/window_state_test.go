package domain_test

import (
	"testing"

	"airspace-acars/internal/domain"
)

func TestDefaultWindowState(t *testing.T) {
	ws := domain.DefaultWindowState()
	if ws.Width != domain.DefaultWindowWidth || ws.Height != domain.DefaultWindowHeight {
		t.Fatalf("first run opens at %dx%d, want %dx%d", ws.Width, ws.Height, domain.DefaultWindowWidth, domain.DefaultWindowHeight)
	}
	if ws.Maximized || ws.Fullscreen || ws.HasPosition {
		t.Fatalf("first run must start normal and uncentred-by-choice, got %+v", ws)
	}
}

// A saved size too small to be a real window is treated as corrupt, never
// honoured.
func TestWindowStateSanitize(t *testing.T) {
	cases := []struct {
		name       string
		in         domain.WindowState
		wantWidth  int
		wantHeight int
	}{
		{"custom bounds are kept", domain.WindowState{Width: 1280, Height: 800}, 1280, 800},
		{"missing size falls back to default", domain.WindowState{}, domain.DefaultWindowWidth, domain.DefaultWindowHeight},
		{"sub-minimum size falls back to default",
			domain.WindowState{Width: domain.MinWindowWidth - 1, Height: domain.MinWindowHeight - 1},
			domain.DefaultWindowWidth, domain.DefaultWindowHeight},
		{"exact minimum is kept",
			domain.WindowState{Width: domain.MinWindowWidth, Height: domain.MinWindowHeight},
			domain.MinWindowWidth, domain.MinWindowHeight},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := tc.in
			ws.Sanitize()
			if ws.Width != tc.wantWidth || ws.Height != tc.wantHeight {
				t.Errorf("got %dx%d, want %dx%d", ws.Width, ws.Height, tc.wantWidth, tc.wantHeight)
			}
		})
	}
}

// Observe is what decides what gets saved. The bug it guards against: a
// maximised window reports the size of the display, and recording that as the
// window's size makes it open display-sized and off to one side next time.
func TestObserveRecordsGeometryOnlyForANormalWindow(t *testing.T) {
	normal := domain.WindowState{Width: 1280, Height: 800, X: 200, Y: 150, HasPosition: true}

	t.Run("a moved and resized window is recorded", func(t *testing.T) {
		got := domain.WindowState{Width: 1100, Height: 700}.Observe(domain.WindowObservation{
			Width: 1280, Height: 800, X: 200, Y: 150,
		})
		if got != normal {
			t.Fatalf("got %+v, want %+v", got, normal)
		}
	})

	t.Run("a maximised window keeps the normal bounds it had", func(t *testing.T) {
		got := normal.Observe(domain.WindowObservation{
			Width: 1936, Height: 1056, X: -8, Y: -8, Maximized: true,
		})
		want := normal
		want.Maximized = true
		if got != want {
			t.Fatalf("display-sized bounds leaked into the saved state:\ngot  %+v\nwant %+v", got, want)
		}
	})

	t.Run("a fullscreen window keeps the normal bounds and wins over maximised", func(t *testing.T) {
		got := normal.Observe(domain.WindowObservation{
			Width: 1920, Height: 1080, Maximized: true, Fullscreen: true,
		})
		want := normal
		want.Fullscreen = true
		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("restoring a maximised window clears the mode and records the restored bounds", func(t *testing.T) {
		maximised := normal
		maximised.Maximized = true
		got := maximised.Observe(domain.WindowObservation{Width: 1000, Height: 640, X: 40, Y: 60})
		want := domain.WindowState{Width: 1000, Height: 640, X: 40, Y: 60, HasPosition: true}
		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("a minimised window changes nothing", func(t *testing.T) {
		maximised := normal
		maximised.Maximized = true
		got := maximised.Observe(domain.WindowObservation{
			Width: 160, Height: 28, X: -32000, Y: -32000, Maximized: true, Minimized: true,
		})
		if got != maximised {
			t.Fatalf("minimising rewrote the state: got %+v, want %+v", got, maximised)
		}
	})

	t.Run("an implausibly small window is not recorded", func(t *testing.T) {
		got := normal.Observe(domain.WindowObservation{Width: 10, Height: 10, X: 5, Y: 5})
		if got != normal {
			t.Fatalf("got %+v, want %+v", got, normal)
		}
	})
}

func TestWindowStateIsVisibleOnScreens(t *testing.T) {
	primary := domain.ScreenBounds{X: 0, Y: 0, Width: 1920, Height: 1080}
	secondary := domain.ScreenBounds{X: 1920, Y: 0, Width: 2560, Height: 1440}

	cases := []struct {
		name    string
		ws      domain.WindowState
		screens []domain.ScreenBounds
		want    bool
	}{
		{"no displays to check permits placement", domain.WindowState{X: 100, Y: 100, Width: 800, Height: 600}, nil, true},
		{"inside the primary display", domain.WindowState{X: 100, Y: 100, Width: 1100, Height: 700}, []domain.ScreenBounds{primary}, true},
		{"on the secondary display", domain.WindowState{X: 2000, Y: 200, Width: 1200, Height: 800}, []domain.ScreenBounds{primary, secondary}, true},
		{"secondary display unplugged", domain.WindowState{X: 2000, Y: 200, Width: 1200, Height: 800}, []domain.ScreenBounds{primary}, false},
		{"far off in negative space", domain.WindowState{X: -2000, Y: -2000, Width: 1100, Height: 700}, []domain.ScreenBounds{primary}, false},
		{"a 50px sliver is not enough", domain.WindowState{X: 1920 - 50, Y: 100, Width: 1100, Height: 700}, []domain.ScreenBounds{primary}, false},
		{"150px overlap is enough", domain.WindowState{X: 1920 - 150, Y: 100, Width: 1100, Height: 700}, []domain.ScreenBounds{primary}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ws.IsVisibleOnScreens(tc.screens); got != tc.want {
				t.Fatalf("IsVisibleOnScreens = %v, want %v", got, tc.want)
			}
		})
	}
}

// FitTo is what keeps a saved placement from ever putting the window
// somewhere the pilot cannot reach.
func TestFitToKeepsTheWindowReachable(t *testing.T) {
	primary := domain.ScreenBounds{X: 0, Y: 0, Width: 1920, Height: 1040, Primary: true}
	secondary := domain.ScreenBounds{X: 1920, Y: 0, Width: 2560, Height: 1400}

	t.Run("a placement that already fits is untouched", func(t *testing.T) {
		in := domain.WindowState{Width: 1100, Height: 700, X: 300, Y: 200, HasPosition: true}
		if got := in.FitTo([]domain.ScreenBounds{primary}); got != in {
			t.Fatalf("got %+v, want %+v", got, in)
		}
	})

	t.Run("no displays leaves the state alone", func(t *testing.T) {
		in := domain.WindowState{Width: 5000, Height: 5000, X: 9, Y: 9, HasPosition: true}
		if got := in.FitTo(nil); got != in {
			t.Fatalf("got %+v, want %+v", got, in)
		}
	})

	// The exact file the maximise bug used to write: display-sized, offset.
	t.Run("a display-sized window at an offset is pulled back onto the display", func(t *testing.T) {
		in := domain.WindowState{Width: 1936, Height: 1056, X: 310, Y: 235, HasPosition: true}
		got := in.FitTo([]domain.ScreenBounds{primary})
		if got.Width != 1920 || got.Height != 1040 || got.X != 0 || got.Y != 0 {
			t.Fatalf("window still spills off the display: %+v", got)
		}
	})

	t.Run("a window hanging off the right edge is moved in", func(t *testing.T) {
		in := domain.WindowState{Width: 1100, Height: 700, X: 1500, Y: 100, HasPosition: true}
		got := in.FitTo([]domain.ScreenBounds{primary})
		if got.X != 1920-1100 || got.Y != 100 {
			t.Fatalf("got (%d, %d), want (%d, 100)", got.X, got.Y, 1920-1100)
		}
	})

	t.Run("it stays on the secondary display it was saved on", func(t *testing.T) {
		in := domain.WindowState{Width: 1200, Height: 800, X: 2200, Y: 300, HasPosition: true}
		if got := in.FitTo([]domain.ScreenBounds{primary, secondary}); got != in {
			t.Fatalf("moved off its display: %+v", got)
		}
	})

	t.Run("an unplugged display drops the position so it is centred", func(t *testing.T) {
		in := domain.WindowState{Width: 1200, Height: 800, X: 2200, Y: 300, HasPosition: true}
		got := in.FitTo([]domain.ScreenBounds{primary})
		if got.HasPosition {
			t.Fatalf("kept an unreachable position: %+v", got)
		}
		if got.Width != 1200 || got.Height != 800 {
			t.Fatalf("size should survive, got %dx%d", got.Width, got.Height)
		}
	})

	t.Run("a window without a position is only capped to the primary display", func(t *testing.T) {
		in := domain.WindowState{Width: 3000, Height: 2000}
		got := in.FitTo([]domain.ScreenBounds{secondary, primary})
		if got.HasPosition || got.Width != 1920 || got.Height != 1040 {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("the mode survives fitting", func(t *testing.T) {
		in := domain.WindowState{Width: 1100, Height: 700, Maximized: true}
		if got := in.FitTo([]domain.ScreenBounds{primary}); !got.Maximized {
			t.Fatalf("fitting dropped the maximised flag: %+v", got)
		}
	})
}
