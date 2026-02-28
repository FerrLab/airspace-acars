package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlightServiceGetBooking(t *testing.T) {
	booking := map[string]interface{}{
		"callsign":  "BAW123",
		"departure": "EGLL",
		"arrival":   "KJFK",
	}

	auth, server := newTestAuthService(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/acars/booking", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		json.NewEncoder(w).Encode(booking)
	})
	defer server.Close()

	flight := &FlightService{auth: auth}

	result, err := flight.GetBooking()
	require.NoError(t, err)
	assert.Equal(t, "BAW123", result["callsign"])
	assert.Equal(t, "EGLL", result["departure"])
	assert.Equal(t, "KJFK", result["arrival"])
}

func TestChatServiceGetMessages(t *testing.T) {
	resp := MessagesResponse{
		Data: []ChatMessage{
			{
				ID:         1,
				SenderName: "Dispatch",
				Type:       "text",
				Message:    "Welcome aboard",
				CreatedAt:  "2025-01-01T00:00:00Z",
			},
		},
		CurrentPage: 1,
		LastPage:    1,
	}

	auth, server := newTestAuthService(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/acars/messages", r.URL.Path)
		assert.Equal(t, "1", r.URL.Query().Get("page"))
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	chat := &ChatService{auth: auth}

	result, err := chat.GetMessages(1)
	require.NoError(t, err)
	require.Len(t, result.Data, 1)
	assert.Equal(t, "Dispatch", result.Data[0].SenderName)
	assert.Equal(t, "Welcome aboard", result.Data[0].Message)
	assert.Equal(t, 1, result.CurrentPage)
	assert.Equal(t, 1, result.LastPage)
}

func TestFlightDataServiceGetFlightDataNow(t *testing.T) {
	expected := sampleFlightData()
	mock := &MockSimConnector{data: expected, name: "MockSim"}

	fds := &FlightDataService{
		connector: mock,
		simActive: true,
	}

	data, err := fds.GetFlightDataNow()
	require.NoError(t, err)
	assert.Equal(t, expected.Position.Latitude, data.Position.Latitude)
	assert.Equal(t, expected.AircraftName, data.AircraftName)
}

func TestFlightDataServiceGetFlightDataNowDisconnected(t *testing.T) {
	fds := &FlightDataService{}

	_, err := fds.GetFlightDataNow()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no simulator connected")
}

func TestChatServiceSendMessage(t *testing.T) {
	auth, server := newTestAuthService(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/acars/message", r.URL.Path)

		var payload map[string]string
		json.NewDecoder(r.Body).Decode(&payload)
		assert.Equal(t, "Hello dispatch", payload["message"])

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(ChatMessage{
			ID:         42,
			SenderName: "Pilot",
			Message:    "Hello dispatch",
			Type:       "text",
			CreatedAt:  "2025-01-01T12:00:00Z",
		})
	})
	defer server.Close()

	chat := &ChatService{auth: auth}

	msg, err := chat.SendMessage("Hello dispatch")
	require.NoError(t, err)
	assert.Equal(t, 42, msg.ID)
	assert.Equal(t, "Hello dispatch", msg.Message)
}

func TestFlightDataServiceConnectedAdapter(t *testing.T) {
	t.Run("returns name when active", func(t *testing.T) {
		mock := &MockSimConnector{name: "SimConnect"}
		fds := &FlightDataService{connector: mock, simActive: true}
		assert.Equal(t, "SimConnect", fds.ConnectedAdapter())
	})

	t.Run("returns empty when inactive", func(t *testing.T) {
		mock := &MockSimConnector{name: "SimConnect"}
		fds := &FlightDataService{connector: mock, simActive: false}
		assert.Equal(t, "", fds.ConnectedAdapter())
	})

	t.Run("returns empty when no connector", func(t *testing.T) {
		fds := &FlightDataService{simActive: true}
		assert.Equal(t, "", fds.ConnectedAdapter())
	})
}

func TestAttemptReconnect(t *testing.T) {
	t.Run("disconnects and reconnects", func(t *testing.T) {
		mock := &ReconnectableMockConnector{
			data: sampleFlightData(),
			name: "TestSim",
		}
		fds := &FlightDataService{connector: mock}

		err := fds.attemptReconnect()
		require.NoError(t, err)
		assert.Equal(t, 1, mock.DisconnectCalls())
		assert.Equal(t, 1, mock.ConnectCalls())
	})

	t.Run("returns error when connect fails", func(t *testing.T) {
		mock := &ReconnectableMockConnector{
			data:       sampleFlightData(),
			name:       "TestSim",
			connectErr: fmt.Errorf("connection refused"),
		}
		fds := &FlightDataService{connector: mock}

		err := fds.attemptReconnect()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection refused")
		assert.Equal(t, 1, mock.DisconnectCalls())
		assert.Equal(t, 1, mock.ConnectCalls())
	})

	t.Run("returns error when no connector", func(t *testing.T) {
		fds := &FlightDataService{}
		err := fds.attemptReconnect()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no connector")
	})
}

func TestDataStreamLoopReconnectsOnFailure(t *testing.T) {
	mock := &ReconnectableMockConnector{
		data: sampleFlightData(),
		name: "TestSim",
	}
	fds := &FlightDataService{
		connector: mock,
		simActive: true,
		streaming: true,
	}
	fds.streamStopCh = make(chan struct{})
	go fds.dataStreamLoop()

	// Let it run successfully for a bit
	time.Sleep(200 * time.Millisecond)
	assert.True(t, fds.IsConnected())

	// Simulate sim disconnection
	mock.SetError(fmt.Errorf("sim crashed"))

	// Wait for reconnect attempt (initial backoff is 2s, but loop ticks every 1s)
	time.Sleep(4 * time.Second)

	// Should have attempted at least one reconnect
	assert.GreaterOrEqual(t, mock.ConnectCalls(), 1, "should have attempted reconnection")
	assert.GreaterOrEqual(t, mock.DisconnectCalls(), 1, "should have disconnected before reconnecting")

	// Now restore the sim
	mock.SetError(nil)

	// Wait for data to be received again
	time.Sleep(3 * time.Second)
	assert.True(t, fds.IsConnected(), "should be reconnected after sim restored")

	close(fds.streamStopCh)
}

func TestDataStreamLoopDoesNotReconnectWhenNeverActive(t *testing.T) {
	mock := &ReconnectableMockConnector{
		name:       "TestSim",
		getDataErr: fmt.Errorf("no data"),
	}
	fds := &FlightDataService{
		connector: mock,
		simActive: false,
		streaming: true,
	}
	fds.streamStopCh = make(chan struct{})
	go fds.dataStreamLoop()

	// Wait a few seconds
	time.Sleep(3 * time.Second)

	// Should NOT attempt reconnect because it was never active
	assert.Equal(t, 0, mock.ConnectCalls(), "should not attempt reconnection when never previously active")
	assert.Equal(t, 0, mock.DisconnectCalls())

	close(fds.streamStopCh)
}

func TestFlightServiceGetBookingServerError(t *testing.T) {
	auth, server := newTestAuthService(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	})
	defer server.Close()

	flight := &FlightService{auth: auth}
	_, err := flight.GetBooking()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse booking")
}

func TestChatServiceGetMessagesWithPagination(t *testing.T) {
	auth, server := newTestAuthService(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		resp := MessagesResponse{CurrentPage: 2, LastPage: 5}
		if page == "2" {
			resp.Data = []ChatMessage{{ID: 10, Message: "Page 2"}}
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	chat := &ChatService{auth: auth}
	result, err := chat.GetMessages(2)
	require.NoError(t, err)
	assert.Equal(t, 2, result.CurrentPage)
	assert.Equal(t, 5, result.LastPage)
	require.Len(t, result.Data, 1)
	assert.Equal(t, "Page 2", result.Data[0].Message)
}
