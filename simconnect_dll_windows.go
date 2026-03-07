//go:build windows

package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

//go:embed SimConnect.dll
var embeddedSimConnectDLL []byte

// ensureSimConnectDLL extracts the embedded SimConnect.dll next to the
// executable if it is missing or does not match the bundled version.
func ensureSimConnectDLL() {
	exePath, err := os.Executable()
	if err != nil {
		slog.Warn("cannot resolve executable path for DLL extraction", "error", err)
		return
	}
	target := filepath.Join(filepath.Dir(exePath), "SimConnect.dll")

	if fileMatchesEmbed(target) {
		return
	}

	slog.Info("extracting embedded SimConnect.dll", "path", target)
	if err := os.WriteFile(target, embeddedSimConnectDLL, 0644); err != nil {
		slog.Warn("failed to extract SimConnect.dll", "error", err)
	}
}

// fileMatchesEmbed returns true if the file at path exists and has the same
// SHA-256 hash as the embedded DLL.
func fileMatchesEmbed(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}

	expected := sha256.Sum256(embeddedSimConnectDLL)
	return bytes.Equal(h.Sum(nil), expected[:])
}
