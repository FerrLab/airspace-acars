# Window State Persistence Design

## 1. Goal

Restore desktop window dimensions, coordinates, and display state (normal, maximized, fullscreen) when Airspace ACARS restarts, while ensuring:
1. Normal window bounds are preserved so that unmaximizing restores the user's custom size and position.
2. Saved coordinates on disconnected monitors gracefully fall back to centering on an active display.
3. Rapid moves or resizes are debounced and never drop state on exit.
4. Clean separation from webview `settings.json` so webview updates do not overwrite desktop window coordinates.

## 2. Approach

- **Domain Model (`internal/domain/window_state.go`)**:
  - `WindowState`: `Width`, `Height`, `X`, `Y`, `Maximized`, `Fullscreen`, `HasPosition`.
  - Minimum bounds enforcement: width >= 600, height >= 400 (defaults to 1100x700).
  - `IsVisibleOnScreens(screens []ScreenBounds) bool`: verifies that at least 100x100 pixels of the window intersect an active monitor work area.
- **State Manager (`window_state.go` in `package main`)**:
  - Stored at `filepath.Join(configDir, "airspace-acars", "window_state.json")`.
  - Loads saved state on launch.
  - Applies geometry and `InitialPosition` (`WindowXY` if position is valid and on-screen, `WindowCentered` otherwise).
  - If `Maximized` or `Fullscreen`, sets appropriate `StartState` and applies mode on launch.
  - Listens to Wails window events:
    - `WindowDidResize` & `WindowDidMove`: updates width, height, x, y when in normal state (`!IsMaximised && !IsFullscreen && !IsMinimised`).
    - `WindowMaximise` & `WindowUnMaximise`: updates `Maximized` flag.
    - `WindowFullscreen` & `WindowUnFullscreen`: updates `Fullscreen` flag.
    - Debounces disk writes (300ms) to prevent I/O thrashing during smooth window dragging.
    - Immediate synchronous flush on `WindowClosing` and `wailsApp.Quit()`.

## 3. Out of Scope

- Storing window placement inside `domain.Settings` (`settings.json`).
- Custom frame chrome or webview window control buttons.
