# Window State Persistence Design

## 1. Goal

Restore the desktop window's size, position and mode (normal, maximized,
fullscreen) when Airspace ACARS restarts, while ensuring:

1. The normal bounds survive a maximized or fullscreen session, so leaving the
   mode returns the pilot to the size and place they chose.
2. A placement on a monitor that is no longer connected, or one that would
   leave part of the window unreachable, is corrected rather than honoured.
3. Rapid moves and resizes do not hammer the disk.
4. The placement is kept apart from `settings.json`, which the webview
   rewrites wholesale and would overwrite coordinates it has never seen.

## 2. Approach

- **Domain (`internal/domain/window_state.go`)** holds the policy as pure
  functions:
  - `WindowState`: `Width`, `Height`, `X`, `Y` (always the *normal* bounds),
    `Maximized`, `Fullscreen`, `HasPosition`.
  - `Observe(WindowObservation)`: folds a reading of the live window into the
    state. The mode always comes from the reading; the geometry only while the
    window is normal. A minimized window is ignored.
  - `FitTo([]ScreenBounds)`: shrinks the window to the display it belongs on
    and pulls it back inside, or drops the position (so it is centred on the
    primary display) when its display is gone.
- **Manager (`window_state.go`, `package main`)** stores the state in
  `<config dir>/airspace-acars/window_state.json` and drives the window.

### Why the window is not started maximized

Wails (Windows) applies `StartState` *before* the position, then moves the
already-maximized window to `(X, Y)`. Starting maximized at a saved position
therefore produced a display-sized window hanging off the screen. Instead, a
window saved maximized or fullscreen is created **hidden, normal, at its saved
normal bounds**, and put into its mode once the webview reports ready
(`WindowRuntimeReady`, with a timeout fallback), then shown. Creating it at the
normal bounds also makes Windows remember them as the restore placement.

### Why events only schedule a read

Event order around a maximize or restore is not something to rely on, and a
maximized window reports the size of the display. Events only (re)arm a 250 ms
debounce. When it fires, the window is read once (mode before and after the
geometry; an unstable reading is retried) and folded in with `Observe`. Nothing
is written unless the state changed, and writes go through a temporary file and
a rename.

A window waiting to be revealed is not observed: reading it would overwrite the
saved mode with "normal" if the pilot quit during startup.

On a real quit the manager is flushed first (`QuitFunc` and the tray's Quit run
off the UI thread, which the read waits on). Closing the window only hides it
to the tray, so nothing needs saving there.

## 3. Span points and logging

Failures are logged at `Warn` with the underlying error only, never the path:
the file sits under the pilot's profile directory. Nothing here raises an
`Error`, so nothing reaches the error tracker.

## 4. Out of scope

- Storing window placement inside `domain.Settings`.
- Per-monitor restore of the normal bounds when the maximized window is moved
  to another display and restored there.
