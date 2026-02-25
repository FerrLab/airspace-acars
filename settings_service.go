package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Settings struct {
	Theme      string `json:"theme"`
	SimType    string `json:"simType"`
	XPlaneHost string `json:"xplaneHost"`
	XPlanePort int    `json:"xplanePort"`
	APIBaseURL string `json:"apiBaseURL"`
}

type SettingsService struct {
	mu       sync.RWMutex
	settings Settings
	filePath string
}

func NewSettingsService() *SettingsService {
	configDir, _ := os.UserConfigDir()
	fp := filepath.Join(configDir, "airspace-acars", "settings.json")

	s := &SettingsService{
		filePath: fp,
		settings: Settings{
			Theme:      "dark",
			SimType:    "auto",
			XPlaneHost: "127.0.0.1",
			XPlanePort: 49000,
			APIBaseURL: "https://airspace.ferrlab.com",
		},
	}
	s.load()
	return s
}

func (s *SettingsService) GetSettings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *SettingsService) UpdateSettings(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
	return s.save()
}

func (s *SettingsService) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.settings)
}

func (s *SettingsService) save() error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	return os.WriteFile(s.filePath, data, 0o644)
}
