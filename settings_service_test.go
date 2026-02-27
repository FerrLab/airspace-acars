package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsDefaults(t *testing.T) {
	s := &SettingsService{
		filePath: filepath.Join(t.TempDir(), "settings.json"),
		settings: Settings{
			Theme:           "dark",
			SimType:         "auto",
			XPlaneHost:      "127.0.0.1",
			XPlanePort:      49000,
			APIBaseURL:      "https://airspace.ferrlab.com",
			ChatSound:       "default",
			DiscordPresence: true,
		},
	}

	got := s.GetSettings()
	assert.Equal(t, "dark", got.Theme)
	assert.Equal(t, "auto", got.SimType)
	assert.Equal(t, "127.0.0.1", got.XPlaneHost)
	assert.Equal(t, 49000, got.XPlanePort)
	assert.Equal(t, "https://airspace.ferrlab.com", got.APIBaseURL)
	assert.Equal(t, "default", got.ChatSound)
	assert.True(t, got.DiscordPresence)
	assert.False(t, got.LocalMode)
}

func TestSettingsSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "sub", "settings.json")

	s := &SettingsService{
		filePath: fp,
		settings: Settings{
			Theme:           "dark",
			SimType:         "auto",
			XPlaneHost:      "127.0.0.1",
			XPlanePort:      49000,
			APIBaseURL:      "https://airspace.ferrlab.com",
			ChatSound:       "default",
			DiscordPresence: true,
		},
	}

	// Update settings
	updated := Settings{
		Theme:           "light",
		SimType:         "xplane",
		XPlaneHost:      "192.168.1.100",
		XPlanePort:      49001,
		APIBaseURL:      "https://custom.example.com",
		LocalMode:       true,
		ChatSound:       "chime",
		DiscordPresence: false,
	}
	require.NoError(t, s.UpdateSettings(updated))

	// Verify in-memory
	assert.Equal(t, updated, s.GetSettings())

	// Load into fresh instance
	s2 := &SettingsService{filePath: fp}
	s2.load()
	assert.Equal(t, updated, s2.GetSettings())
}

func TestSettingsLoadNonExistentFile(t *testing.T) {
	s := &SettingsService{
		filePath: filepath.Join(t.TempDir(), "nonexistent.json"),
		settings: Settings{Theme: "dark"},
	}
	s.load() // should not panic or error
	assert.Equal(t, "dark", s.GetSettings().Theme)
}
