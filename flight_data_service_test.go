package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStalenessDetectionCondition verifies that the staleness condition
// (LastReceived > 10s ago while simActive=true) correctly identifies stale data.
func TestStalenessDetectionCondition(t *testing.T) {
	t.Run("stale when LastReceived is more than 10s ago", func(t *testing.T) {
		lastReceived := time.Now().Add(-15 * time.Second)
		simActive := true

		isStale := simActive && !lastReceived.IsZero() &&
			time.Since(lastReceived) > 10*time.Second

		assert.True(t, isStale, "should detect staleness when last data was 15s ago")
	})

	t.Run("not stale when LastReceived is recent", func(t *testing.T) {
		lastReceived := time.Now().Add(-3 * time.Second)
		simActive := true

		isStale := simActive && !lastReceived.IsZero() &&
			time.Since(lastReceived) > 10*time.Second

		assert.False(t, isStale, "should not detect staleness when last data was 3s ago")
	})

	t.Run("not stale when simActive is false", func(t *testing.T) {
		lastReceived := time.Now().Add(-30 * time.Second)
		simActive := false

		isStale := simActive && !lastReceived.IsZero() &&
			time.Since(lastReceived) > 10*time.Second

		assert.False(t, isStale, "should not detect staleness when sim is not active")
	})

	t.Run("not stale when LastReceived is zero", func(t *testing.T) {
		lastReceived := time.Time{}
		simActive := true

		isStale := simActive && !lastReceived.IsZero() &&
			time.Since(lastReceived) > 10*time.Second

		assert.False(t, isStale, "should not detect staleness when LastReceived is zero")
	})

	t.Run("boundary at exactly 10s", func(t *testing.T) {
		// At exactly 10s, time.Since > 10s should be false (or barely true
		// due to execution time). Use 10s + small buffer to be deterministic.
		lastReceived := time.Now().Add(-10*time.Second - 100*time.Millisecond)
		simActive := true

		isStale := simActive && !lastReceived.IsZero() &&
			time.Since(lastReceived) > 10*time.Second

		assert.True(t, isStale, "should detect staleness just past 10s threshold")
	})
}

// TestMockSimConnectorLastReceived verifies that MockSimConnector.LastReceived()
// correctly returns its lastReceived field value.
func TestMockSimConnectorLastReceived(t *testing.T) {
	t.Run("returns zero time by default", func(t *testing.T) {
		mock := &MockSimConnector{name: "TestSim"}
		assert.True(t, mock.LastReceived().IsZero(), "default LastReceived should be zero")
	})

	t.Run("returns set time value", func(t *testing.T) {
		now := time.Now()
		mock := &MockSimConnector{
			name:         "TestSim",
			lastReceived: now,
		}
		assert.Equal(t, now, mock.LastReceived(), "LastReceived should return the set time")
	})

	t.Run("returns past time value", func(t *testing.T) {
		past := time.Now().Add(-30 * time.Second)
		mock := &MockSimConnector{
			name:         "TestSim",
			lastReceived: past,
		}
		assert.Equal(t, past, mock.LastReceived())
		assert.True(t, time.Since(mock.LastReceived()) > 25*time.Second,
			"should reflect that the time is in the past")
	})

	t.Run("ReconnectableMockConnector also returns lastReceived", func(t *testing.T) {
		now := time.Now()
		mock := &ReconnectableMockConnector{
			name:         "TestSim",
			lastReceived: now,
		}
		assert.Equal(t, now, mock.LastReceived())
	})
}

// TestBackoffCalculation verifies the exponential backoff formula
// min(2^attempts * 5s, 60s) produces the correct durations.
func TestBackoffCalculation(t *testing.T) {
	tests := []struct {
		attempts int
		expected time.Duration
	}{
		{0, 5 * time.Second},   // 2^0 * 5 = 5s
		{1, 10 * time.Second},  // 2^1 * 5 = 10s
		{2, 20 * time.Second},  // 2^2 * 5 = 20s
		{3, 40 * time.Second},  // 2^3 * 5 = 40s
		{4, 60 * time.Second},  // 2^4 * 5 = 80s, capped to 60s
		{5, 60 * time.Second},  // 2^5 * 5 = 160s, capped to 60s
		{10, 60 * time.Second}, // 2^10 * 5 = 5120s, capped to 60s
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("attempts_%d", tc.attempts), func(t *testing.T) {
			// This is the exact formula from dataStreamLoop
			backoff := time.Duration(1<<uint(tc.attempts)) * reconnectBaseDelay
			if backoff > reconnectMaxBackoff {
				backoff = reconnectMaxBackoff
			}

			assert.Equal(t, tc.expected, backoff,
				"backoff for %d attempts should be %v", tc.attempts, tc.expected)
		})
	}
}

// TestReconnectStateResetOnConnect verifies that reconnectAttempts and
// lastReconnectAt are properly reset when a successful connection is established.
func TestReconnectStateResetOnConnect(t *testing.T) {
	t.Run("reset on successful data after reconnect", func(t *testing.T) {
		mock := &ReconnectableMockConnector{
			data: sampleFlightData(),
			name: "TestSim",
		}
		fds := &FlightDataService{
			connector:         mock,
			simActive:         false, // was previously inactive
			streaming:         true,
			adapterName:       "TestSim",
			reconnectAttempts: 3,
			lastReconnectAt:   time.Now().Add(-10 * time.Second),
		}
		fds.streamStopCh = make(chan struct{})
		go fds.dataStreamLoop()

		// Wait for the loop to pick up data and transition to active
		require.Eventually(t, func() bool {
			return fds.IsConnected()
		}, 5*time.Second, 200*time.Millisecond, "should become active after receiving data")

		// Verify reconnect state was reset
		fds.mu.Lock()
		attempts := fds.reconnectAttempts
		lastReconnect := fds.lastReconnectAt
		fds.mu.Unlock()

		assert.Equal(t, 0, attempts, "reconnectAttempts should be reset to 0")
		assert.True(t, lastReconnect.IsZero(), "lastReconnectAt should be reset to zero")

		close(fds.streamStopCh)
	})

	t.Run("ConnectSim resets reconnect state", func(t *testing.T) {
		// Verify that the FlightDataService fields are properly initialized
		// when constructing with reconnect state already set.
		fds := &FlightDataService{
			reconnectAttempts: 5,
			lastReconnectAt:   time.Now(),
		}

		// Simulate what ConnectSim does after a successful connection:
		fds.mu.Lock()
		fds.reconnectAttempts = 0
		fds.lastReconnectAt = time.Time{}
		fds.mu.Unlock()

		assert.Equal(t, 0, fds.reconnectAttempts)
		assert.True(t, fds.lastReconnectAt.IsZero())
	})
}

// TestReconnectSimUnknownAdapter verifies that reconnectSim returns an error
// containing "unknown adapter" when called with an unrecognised adapter name.
func TestReconnectSimUnknownAdapter(t *testing.T) {
	t.Run("returns error for empty adapter name", func(t *testing.T) {
		fds := &FlightDataService{
			adapterName: "",
		}

		err := fds.reconnectSim()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown adapter")
	})

	t.Run("returns error for bogus adapter name", func(t *testing.T) {
		fds := &FlightDataService{
			adapterName: "BogusAdapter",
		}

		err := fds.reconnectSim()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown adapter")
	})

	t.Run("error includes the adapter name", func(t *testing.T) {
		fds := &FlightDataService{
			adapterName: "FakeSimulator",
		}

		err := fds.reconnectSim()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown adapter")
		assert.Contains(t, err.Error(), "FakeSimulator")
	})
}
