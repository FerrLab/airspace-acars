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
			Language:        "en",
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

func TestAirportField(t *testing.T) {
	d := newTestDiscordService()

	tests := []struct {
		name       string
		booking    map[string]interface{}
		airportKey string
		field      string
		want       string
	}{
		{
			name: "extracts icao from nested airport",
			booking: map[string]interface{}{
				"departure_airport": map[string]interface{}{"icao": "EGLL", "city": "London"},
			},
			airportKey: "departure_airport",
			field:      "icao",
			want:       "EGLL",
		},
		{
			name: "extracts city from nested airport",
			booking: map[string]interface{}{
				"departure_airport": map[string]interface{}{"icao": "EGLL", "city": "London"},
			},
			airportKey: "departure_airport",
			field:      "city",
			want:       "London",
		},
		{
			name:       "empty when airport key missing",
			booking:    map[string]interface{}{"other": "value"},
			airportKey: "departure_airport",
			field:      "icao",
			want:       "",
		},
		{
			name:       "empty when booking is nil",
			booking:    nil,
			airportKey: "departure_airport",
			field:      "icao",
			want:       "",
		},
		{
			name: "empty when airport is not an object",
			booking: map[string]interface{}{
				"departure_airport": "EGLL",
			},
			airportKey: "departure_airport",
			field:      "icao",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.airportField(tt.booking, tt.airportKey, tt.field)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCityForFlight(t *testing.T) {
	d := newTestDiscordService()

	t.Run("returns city from booking cache", func(t *testing.T) {
		d.bookingCache = map[string]interface{}{
			"departure_airport": map[string]interface{}{"icao": "EGLL", "city": "London"},
		}
		assert.Equal(t, "London", d.cityForFlight("departure_airport", "EGLL"))
	})

	t.Run("falls back to code when city not in cache", func(t *testing.T) {
		d.bookingCache = map[string]interface{}{
			"departure_airport": map[string]interface{}{"icao": "EGLL"},
		}
		assert.Equal(t, "EGLL", d.cityForFlight("departure_airport", "EGLL"))
	})

	t.Run("returns unknown when no cache and no code", func(t *testing.T) {
		d.bookingCache = nil
		assert.Equal(t, "unknown", d.cityForFlight("departure_airport", ""))
	})

	t.Run("returns code when cache is nil", func(t *testing.T) {
		d.bookingCache = nil
		assert.Equal(t, "KJFK", d.cityForFlight("arrival_airport", "KJFK"))
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
		assert.Equal(t, fmt.Sprintf(idlePhrasesI18n["en"][0], "London"), got)
	})

	t.Run("rotates after 2 minutes", func(t *testing.T) {
		d.phraseIdx = 0
		d.phraseTime = time.Now().Add(-3 * time.Minute) // older than 2 min
		_ = d.idlePhrase("London")
		assert.Equal(t, 1, d.phraseIdx)
	})

	t.Run("wraps phrase index", func(t *testing.T) {
		d.phraseIdx = len(idlePhrasesI18n["en"]) - 1
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
		d.flight.departure = "EGLL"
		d.flight.arrival = "KJFK"
		d.flight.startTime = time.Now().Add(-30 * time.Minute)
		d.bookingCache = map[string]interface{}{
			"departure_airport": map[string]interface{}{"icao": "EGLL", "city": "London"},
			"arrival_airport":   map[string]interface{}{"icao": "KJFK", "city": "New York"},
		}

		activity := d.buildActivity("Airline Co", "https://logo.png")

		assert.Equal(t, "Airline Co — BAW123", activity["details"])
		assert.Equal(t, "Flying London → New York", activity["state"])
		require.Contains(t, activity, "timestamps")
		require.Contains(t, activity, "assets")
		assets := activity["assets"].(map[string]interface{})
		assert.Equal(t, "https://logo.png", assets["large_image"])
	})

	t.Run("idle with booking", func(t *testing.T) {
		d := newTestDiscordService()
		d.flight.state = "idle"
		d.bookingCache = map[string]interface{}{
			"departure_airport": map[string]interface{}{"icao": "EGLL", "city": "London"},
		}
		d.bookingCacheTime = time.Now()
		d.phraseIdx = 0
		d.phraseTime = time.Now()

		activity := d.buildActivity("Airline Co", "")

		assert.Equal(t, "Airline Co", activity["details"])
		assert.Equal(t, fmt.Sprintf(idlePhrasesI18n["en"][0], "London"), activity["state"])
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
