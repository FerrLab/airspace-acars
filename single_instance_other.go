//go:build !linux

package main

// platformSingleInstanceLockAvailable reports true: Wails keeps the lock in a
// named mutex on Windows and a lock file on macOS, and neither depends on a
// service that may be missing.
func platformSingleInstanceLockAvailable() bool { return true }
