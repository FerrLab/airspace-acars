package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDiscordService() *DiscordService {
	settings := &SettingsService{
		settings: Settings{
			APIBaseURL:      "http://localhost",
			DiscordPresence: true,
		},
	}
	auth := &AuthService{
		httpClient: http.DefaultClient,
		settings:   settings,
	}
	flight := &FlightService{
		auth:  auth,
		state: "idle",
	}
	return &DiscordService{
		settings:   settings,
		auth:       auth,
		flight:     flight,
		phraseIdx:  0,
		phraseTime: time.Now(),
	}
}

func TestBookingField(t *testing.T) {
	d := newTestDiscordService()

	tests := []struct {
		name    string
		booking map[string]interface{}
		keys    []string
		want    string
	}{
		{
			name:    "first key found",
			booking: map[string]interface{}{"departure": "EGLL", "dep": "LHR"},
			keys:    []string{"departure", "dep"},
			want:    "EGLL",
		},
		{
			name:    "falls back to second key",
			booking: map[string]interface{}{"dep": "LHR"},
			keys:    []string{"departure", "dep"},
			want:    "LHR",
		},
		{
			name:    "empty when no keys match",
			booking: map[string]interface{}{"other": "value"},
			keys:    []string{"departure", "dep"},
			want:    "",
		},
		{
			name:    "skips empty string values",
			booking: map[string]interface{}{"departure": "", "dep": "LHR"},
			keys:    []string{"departure", "dep"},
			want:    "LHR",
		},
		{
			name:    "skips non-string values",
			booking: map[string]interface{}{"departure": 42, "dep": "LHR"},
			keys:    []string{"departure", "dep"},
			want:    "LHR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.bookingField(tt.booking, tt.keys...)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCityFromBooking(t *testing.T) {
	d := newTestDiscordService()

	t.Run("returns city from booking cache", func(t *testing.T) {
		d.bookingCache = map[string]interface{}{
			"departure_city": "London",
		}
		assert.Equal(t, "London", d.cityFromBooking("departure", "EGLL"))
	})

	t.Run("falls back to code when city not in cache", func(t *testing.T) {
		d.bookingCache = map[string]interface{}{}
		assert.Equal(t, "EGLL", d.cityFromBooking("departure", "EGLL"))
	})

	t.Run("returns unknown when no cache and no code", func(t *testing.T) {
		d.bookingCache = nil
		assert.Equal(t, "unknown", d.cityFromBooking("departure", ""))
	})

	t.Run("returns code when cache is nil", func(t *testing.T) {
		d.bookingCache = nil
		assert.Equal(t, "KJFK", d.cityFromBooking("arrival", "KJFK"))
	})
}

func TestIdlePhrase(t *testing.T) {
	d := newTestDiscordService()

	t.Run("returns dispatch message for empty city", func(t *testing.T) {
		assert.Equal(t, "Standing by for dispatch", d.idlePhrase(""))
	})

	t.Run("formats phrase with city", func(t *testing.T) {
		d.phraseIdx = 0
		d.phraseTime = time.Now()
		got := d.idlePhrase("London")
		assert.Equal(t, fmt.Sprintf(idlePhrases[0], "London"), got)
	})

	t.Run("rotates after 2 minutes", func(t *testing.T) {
		d.phraseIdx = 0
		d.phraseTime = time.Now().Add(-3 * time.Minute) // older than 2 min
		_ = d.idlePhrase("London")
		assert.Equal(t, 1, d.phraseIdx)
	})

	t.Run("wraps phrase index", func(t *testing.T) {
		d.phraseIdx = len(idlePhrases) - 1
		d.phraseTime = time.Now().Add(-3 * time.Minute)
		_ = d.idlePhrase("London")
		assert.Equal(t, 0, d.phraseIdx)
	})
}

func TestBuildActivity(t *testing.T) {
	t.Run("active flight", func(t *testing.T) {
		d := newTestDiscordService()
		d.flight.state = "active"
		d.flight.callsign = "BAW123"
		d.flight.arrival = "KJFK"
		d.flight.startTime = time.Now().Add(-30 * time.Minute)
		d.bookingCache = map[string]interface{}{
			"arrival_city": "New York",
		}

		activity := d.buildActivity("Airline Co", "https://logo.png")

		assert.Equal(t, "Airline Co — BAW123", activity["details"])
		assert.Equal(t, "Flying to New York", activity["state"])
		require.Contains(t, activity, "timestamps")
		require.Contains(t, activity, "assets")
		assets := activity["assets"].(map[string]interface{})
		assert.Equal(t, "https://logo.png", assets["large_image"])
	})

	t.Run("idle with booking", func(t *testing.T) {
		d := newTestDiscordService()
		d.flight.state = "idle"
		d.bookingCache = map[string]interface{}{
			"departure":      "EGLL",
			"departure_city": "London",
		}
		d.bookingCacheTime = time.Now()
		d.phraseIdx = 0
		d.phraseTime = time.Now()

		activity := d.buildActivity("Airline Co", "")

		assert.Equal(t, "Airline Co", activity["details"])
		assert.Equal(t, fmt.Sprintf(idlePhrases[0], "London"), activity["state"])
		assert.NotContains(t, activity, "timestamps")
		assert.NotContains(t, activity, "assets") // no logo
	})

	t.Run("idle without booking", func(t *testing.T) {
		d := newTestDiscordService()
		d.flight.state = "idle"
		d.bookingCache = nil
		// Force booking fetch to fail by having no auth set up
		d.bookingCacheTime = time.Time{} // expired

		activity := d.buildActivity("Airline Co", "https://logo.png")

		assert.Equal(t, "Airline Co", activity["details"])
		assert.Equal(t, "Standing by for dispatch", activity["state"])
	})
}
