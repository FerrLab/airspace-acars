//go:build windows

package main

import (
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"
)

//go:embed SimConnect.dll
var embeddedSimConnectDLL []byte

// ensureSimConnectDLL extracts the embedded SimConnect.dll next to the
// executable only if no DLL is present there yet. An existing DLL (whether
// placed by the installer or the MSFS SDK) is never overwritten — it may
// be a version matched to the user's simulator.
func ensureSimConnectDLL() {
	exePath, err := os.Executable()
	if err != nil {
		slog.Warn("cannot resolve executable path for DLL extraction", "error", err)
		return
	}
	target := filepath.Join(filepath.Dir(exePath), "SimConnect.dll")

	if _, err := os.Stat(target); err == nil {
		return // DLL already exists — leave it alone
	}

	slog.Info("extracting embedded SimConnect.dll", "path", target)
	if err := os.WriteFile(target, embeddedSimConnectDLL, 0644); err != nil {
		slog.Warn("failed to extract SimConnect.dll", "error", err)
	}
}
