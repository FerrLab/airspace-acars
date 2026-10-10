package domain

const (
	// DefaultWindowWidth is the out-of-the-box window width.
	DefaultWindowWidth = 1100

	// DefaultWindowHeight is the out-of-the-box window height.
	DefaultWindowHeight = 700

	// MinWindowWidth keeps the window usable and legible. A saved width below
	// it is treated as corrupt rather than honoured.
	MinWindowWidth = 600

	// MinWindowHeight keeps the window usable and legible. A saved height
	// below it is treated as corrupt rather than honoured.
	MinWindowHeight = 400

	// minVisibleDimension is how many pixels of a saved window must overlap a
	// display, on each axis, for that display to count as the one the window
	// belongs on. Enough to see and grab the title bar; a smaller sliver means
	// the monitor the window lived on is gone.
	minVisibleDimension = 100
)

// WindowState is the placement of the desktop window that is restored on the
// next launch.
//
// Width, Height, X and Y always describe the window in its normal (restored)
// state, even while Maximized or Fullscreen is set, so that leaving either
// mode returns the pilot to the size and place they chose rather than to the
// default.
type WindowState struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	X      int `json:"x"`
	Y      int `json:"y"`
	// Maximized is set when the window was closed maximised.
	Maximized bool `json:"maximized"`
	// Fullscreen is set when the window was closed fullscreen. It wins over
	// Maximized.
	Fullscreen bool `json:"fullscreen"`
	// HasPosition tells a placement the pilot chose from the zero value
	// (0, 0), which is a legitimate position but also what a first run has.
	HasPosition bool `json:"has_position"`
}

// WindowObservation is what the windowing system reports about the live
// window at one instant.
type WindowObservation struct {
	Width, Height, X, Y int
	Maximized           bool
	Fullscreen          bool
	Minimized           bool
}

// ScreenBounds is the usable rectangle of one display, in the same
// coordinate space as the window's X and Y.
type ScreenBounds struct {
	X, Y, Width, Height int
	// Primary marks the display a window is centred on when it has no
	// usable saved position.
	Primary bool
}

// DefaultWindowState returns the placement used on a first run.
func DefaultWindowState() WindowState {
	return WindowState{
		Width:  DefaultWindowWidth,
		Height: DefaultWindowHeight,
	}
}

// Sanitize replaces dimensions that are missing or too small to be a real
// window, as found in a hand-edited or truncated file.
func (s *WindowState) Sanitize() {
	if s.Width < MinWindowWidth {
		s.Width = DefaultWindowWidth
	}
	if s.Height < MinWindowHeight {
		s.Height = DefaultWindowHeight
	}
}

// Observe folds a reading of the live window into the state and returns the
// result.
//
// The window's mode is always taken from the reading. Its geometry is taken
// only while the window is in its normal state: a maximised or fullscreen
// window reports the size of the display, and recording that is exactly what
// would make the window open display-sized and off to one side next time. A
// minimised window is ignored altogether, because Windows parks it at
// (-32000, -32000) and keeps its previous mode.
func (s WindowState) Observe(o WindowObservation) WindowState {
	if o.Minimized {
		return s
	}
	s.Fullscreen = o.Fullscreen
	s.Maximized = o.Maximized && !o.Fullscreen
	if o.Maximized || o.Fullscreen {
		return s
	}
	if o.Width < MinWindowWidth || o.Height < MinWindowHeight {
		return s
	}
	s.Width, s.Height = o.Width, o.Height
	s.X, s.Y = o.X, o.Y
	s.HasPosition = true
	return s
}

// IsVisibleOnScreens reports whether the window overlaps at least one display
// by minVisibleDimension pixels on each axis. With no displays to check
// against (a headless run) it reports true.
func (s WindowState) IsVisibleOnScreens(screens []ScreenBounds) bool {
	if len(screens) == 0 {
		return true
	}
	_, ok := s.screenWithMostOverlap(screens)
	return ok
}

// FitTo returns the state adjusted so that showing it can never put the
// window somewhere the pilot cannot reach.
//
// The window is shrunk to the display it belongs on and pulled back inside
// it, which also repairs a file that recorded a display-sized window at an
// offset. When the display it was saved on is gone, the position is dropped
// (HasPosition false) so the caller centres it on the primary display. With
// no displays to check against the state is returned unchanged.
func (s WindowState) FitTo(screens []ScreenBounds) WindowState {
	if len(screens) == 0 {
		return s
	}

	var target ScreenBounds
	if s.HasPosition {
		var ok bool
		if target, ok = s.screenWithMostOverlap(screens); !ok {
			s.HasPosition = false
			target = primaryScreen(screens)
		}
	} else {
		target = primaryScreen(screens)
	}

	s.Width = min(s.Width, target.Width)
	s.Height = min(s.Height, target.Height)
	if s.HasPosition {
		s.X = max(target.X, min(s.X, target.X+target.Width-s.Width))
		s.Y = max(target.Y, min(s.Y, target.Y+target.Height-s.Height))
	}
	return s
}

// screenWithMostOverlap picks the display the saved rectangle sits on. It
// reports false when none overlaps by minVisibleDimension on both axes.
func (s WindowState) screenWithMostOverlap(screens []ScreenBounds) (ScreenBounds, bool) {
	var best ScreenBounds
	bestArea := 0
	for _, sc := range screens {
		w := min(s.X+s.Width, sc.X+sc.Width) - max(s.X, sc.X)
		h := min(s.Y+s.Height, sc.Y+sc.Height) - max(s.Y, sc.Y)
		if w < minVisibleDimension || h < minVisibleDimension {
			continue
		}
		if area := w * h; area > bestArea {
			best, bestArea = sc, area
		}
	}
	return best, bestArea > 0
}

// primaryScreen returns the display marked primary, or the first one when
// the platform marks none.
func primaryScreen(screens []ScreenBounds) ScreenBounds {
	for _, sc := range screens {
		if sc.Primary {
			return sc
		}
	}
	return screens[0]
}
