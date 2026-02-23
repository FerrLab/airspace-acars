package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"
)

type mockDevice struct {
	userCode  string
	approved  bool
	createdAt time.Time
}

type MockAuthServer struct {
	mu      sync.Mutex
	devices map[string]*mockDevice
	addr    string
}

func NewMockAuthServer() *MockAuthServer {
	return &MockAuthServer{
		devices: make(map[string]*mockDevice),
	}
}

func (m *MockAuthServer) Start() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen: %w", err)
	}
	m.addr = listener.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /device/code", m.handleDeviceCode)
	mux.HandleFunc("POST /device/token", m.handleDeviceToken)
	mux.HandleFunc("GET /activate", m.handleActivate)

	go func() {
		slog.Info("mock auth server started", "addr", m.addr)
		http.Serve(listener, mux)
	}()

	return m.addr, nil
}

func (m *MockAuthServer) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	deviceCode := randomCode(32)
	userCode := randomCode(6)

	dev := &mockDevice{
		userCode:  userCode,
		createdAt: time.Now(),
	}
	m.devices[deviceCode] = dev

	// Auto-approve after 15 seconds
	go func() {
		time.Sleep(15 * time.Second)
		m.mu.Lock()
		defer m.mu.Unlock()
		if d, ok := m.devices[deviceCode]; ok {
			d.approved = true
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device_code":      deviceCode,
		"user_code":        userCode,
		"verification_uri": fmt.Sprintf("http://%s/activate", m.addr),
		"expires_in":       300,
		"interval":         5,
	})
}

func (m *MockAuthServer) handleDeviceToken(w http.ResponseWriter, r *http.Request) {
	deviceCode := r.FormValue("device_code")

	m.mu.Lock()
	dev, ok := m.devices[deviceCode]
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_device_code"})
		return
	}

	if !dev.approved {
		json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": "mock-token-" + randomCode(16),
		"token_type":   "Bearer",
	})
}

func (m *MockAuthServer) handleActivate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html><html><body style="font-family:sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#0a0a0a;color:#fafafa">
	<div style="text-align:center"><h1>Airspace ACARS</h1><p>Device will be authorized automatically in a few seconds.</p></div></body></html>`)
}

const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
