package domain

// WindowState represents the persisted geometry and display state of the desktop window.
type WindowState struct {
	Width       int  `json:"width"`
	Height      int  `json:"height"`
	X           int  `json:"x"`
	Y           int  `json:"y"`
	Maximized   bool `json:"maximized"`
	Fullscreen  bool `json:"fullscreen"`
	HasPosition bool `json:"has_position"`
}

const (
	// DefaultWindowWidth is the out-of-the-box window width.
	DefaultWindowWidth = 1100

	// DefaultWindowHeight is the out-of-the-box window height.
	DefaultWindowHeight = 700

	// MinWindowWidth ensures the window stays usable and legible.
	MinWindowWidth = 600

	// MinWindowHeight ensures the window stays usable and legible.
	MinWindowHeight = 400

	// minVisibleDimension is the minimum width and height in pixels that must
	// intersect an active display work area for a saved position to be considered valid.
	minVisibleDimension = 100
)

// DefaultWindowState returns standard default window parameters.
func DefaultWindowState() WindowState {
	return WindowState{
		Width:       DefaultWindowWidth,
		Height:      DefaultWindowHeight,
		X:           0,
		Y:           0,
		Maximized:   false,
		Fullscreen:  false,
		HasPosition: false,
	}
}

// Sanitize ensures window dimensions respect minimum sizing constraints.
func (s *WindowState) Sanitize() {
	if s.Width < MinWindowWidth {
		s.Width = DefaultWindowWidth
	}
	if s.Height < MinWindowHeight {
		s.Height = DefaultWindowHeight
	}
}

// ScreenBounds represents the usable coordinate rectangle of a physical monitor.
type ScreenBounds struct {
	X      int
	Y      int
	Width  int
	Height int
}

// IsVisibleOnScreens returns true if the window bounds intersect at least one active screen
// by at least minVisibleDimension pixels in each dimension.
// If screens is empty, returns true to allow headless or virtual display execution.
func (s WindowState) IsVisibleOnScreens(screens []ScreenBounds) bool {
	if len(screens) == 0 {
		return true
	}

	for _, sc := range screens {
		// Calculate overlap along X and Y axes
		left := max(s.X, sc.X)
		right := min(s.X+s.Width, sc.X+sc.Width)
		top := max(s.Y, sc.Y)
		bottom := min(s.Y+s.Height, sc.Y+sc.Height)

		overlapW := right - left
		overlapH := bottom - top

		if overlapW >= minVisibleDimension && overlapH >= minVisibleDimension {
			return true
		}
	}

	return false
}
