//go:build linux

package main

import (
	"log/slog"

	"github.com/godbus/dbus/v5"
)

// platformSingleInstanceLockAvailable reports whether the D-Bus session bus,
// where Wails keeps its single-instance lock on Linux, can be reached.
// Without one Wails would stop the application at startup; the ACARS starts
// unguarded instead, as its system tray already carries on without one.
func platformSingleInstanceLockAvailable() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		slog.Warn("no d-bus session bus; a second launch will open another copy", "error", err)
		return false
	}
	// Only probing: Wails opens its own connection for the lock, and a
	// failed close leaves nothing behind that matters.
	_ = conn.Close()
	return true
}
