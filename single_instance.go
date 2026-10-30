package main

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// singleInstanceID names the lock that tells a second launch the ACARS is
// already open. Wails turns it into a named mutex on Windows, a lock file on
// macOS and a D-Bus name on Linux. Builds with different IDs do not see each
// other, so changing it lets two copies run at once.
const singleInstanceID = "com.ferrlab.airspace-acars"

// mainWindowName is the name the ACARS window is registered under, so a
// second launch can find it.
const mainWindowName = "main"

// singleInstanceLockAvailable reports whether this machine can hold the
// single-instance lock. It is a variable so tests can stand in for the
// platform; see withSingleInstanceLock.
var singleInstanceLockAvailable = platformSingleInstanceLockAvailable

// singleInstance returns the options that make a second launch hand over to
// the copy already running and exit, or nil when the lock cannot be held
// here and the ACARS should start unguarded rather than not at all.
//
// It replaces a listener on 127.0.0.1:49876, which took any failure to
// listen for a running copy. That port is in Windows' dynamic range, where
// Hyper-V, WSL2 and Docker reserve blocks at boot; when a block covered it,
// every launch exited before opening its log and the ACARS would not start
// until a reboot moved the reservation.
func singleInstance(show func()) *application.SingleInstanceOptions {
	if !singleInstanceLockAvailable() {
		return nil
	}
	return &application.SingleInstanceOptions{
		UniqueID: singleInstanceID,
		OnSecondInstanceLaunch: func(application.SecondInstanceData) {
			slog.Info("acars launched again; showing the open window")
			show()
		},
	}
}

// showMainWindow brings the ACARS window forward. It looks the window up
// rather than capturing it because the second-launch callback is handed to
// Wails before the window exists.
func showMainWindow() {
	w, ok := application.Get().Window.GetByName(mainWindowName)
	if !ok {
		return
	}
	w.Show()
	w.Focus()
}
