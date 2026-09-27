package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"airspace-acars/internal/domain"
)

// InitSettings loads settings from disk or creates defaults.
func (a *App) InitSettings() {
	configDir, _ := os.UserConfigDir()
	a.settingsPath = filepath.Join(configDir, "airspace-acars", "settings.json")
	a.settings = domain.DefaultSettings()

	data, err := os.ReadFile(a.settingsPath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &a.settings)
}

// GetSettings returns the current settings.
func (a *App) GetSettings() domain.Settings {
	a.settingsMu.RLock()
	defer a.settingsMu.RUnlock()
	return a.settings
}

// UpdateSettings persists new settings.
func (a *App) UpdateSettings(settings domain.Settings) error {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	a.settings = settings
	return a.saveSettings()
}

func (a *App) saveSettings() error {
	dir := filepath.Dir(a.settingsPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(a.settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	return os.WriteFile(a.settingsPath, data, 0o644)
}
