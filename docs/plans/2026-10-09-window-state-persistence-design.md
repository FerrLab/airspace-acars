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
    primary display).
  - `Centred([]ScreenBounds)`: places a window without a position in the
    middle of the primary display, the one `FitTo` sized it for. Wails' own
    `Center` uses the display nearest the window, which after an unplug is
    the dead display's neighbour and may be smaller than that size.
- **Manager (`internal/adapters/window`)** stores the state in
  `<config dir>/airspace-acars/window_state.json` and drives the window.

### Why windows with saved placement start hidden

Wails (Windows) populates its screen list only inside `Run()`. Reading
`Screen.GetAll()` before `Run()` returns an empty list, so fitting the window to
active displays must happen after the runtime is ready. In addition, Wails
applies `StartState` *before* the position, which would produce a display-sized
window hanging off the screen if started maximized.

Instead, any window with a saved position or mode is created **hidden, normal,
at its saved bounds**, fitted to the connected screens on `WindowRuntimeReady`
(with bounds set, or centered if its display is disconnected), put into its mode
(if maximized or fullscreen), and then shown. Creating it at the normal bounds
also makes Windows remember them as the restore placement.

The tray's Show, a second launch and the close prompt also go through the
reveal, so that a click before `WindowRuntimeReady` still fits the window. Once
the window is being recorded, the reveal does nothing: a window that started
visible is where the pilot has since put it, and setting the saved bounds on it
again would shrink a maximized window and grow it back.

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
