//go:build !windows

package main

// embeddedSimConnectDLL is nil on non-Windows platforms.
// SimConnect is only available on Windows.
var embeddedSimConnectDLL []byte
