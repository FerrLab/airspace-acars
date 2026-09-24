package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// captureLogs redirects slog for the duration of a test and returns the
// records as they were written.
func captureLogs(t *testing.T) func() []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return func() []map[string]any {
		var out []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			if line == "" {
				continue
			}
			var rec map[string]any
			if err := json.Unmarshal([]byte(line), &rec); err == nil {
				out = append(out, rec)
			}
		}
		return out
	}
}

func messages(records []map[string]any) []string {
	out := make([]string, 0, len(records))
	for _, r := range records {
		out = append(out, r["msg"].(string))
	}
	return out
}

// A pilot whose simulator is not running produced a log that stopped after
// "adapter opened, waiting for data": the line explaining why was at Debug,
// which is not in a pilot's log. That log was indistinguishable from one where
// the ACARS itself was broken.
func TestSimulatorSilenceIsReported(t *testing.T) {
	logs := captureLogs(t)
	a := &App{}

	a.noteSimWaitFailed("X-Plane")

	records := logs()
	if len(records) != 1 {
		t.Fatalf("logged %d records, want 1: %v", len(records), messages(records))
	}
	if got := records[0]["level"]; got != "WARN" {
		t.Errorf("level = %v, want WARN — Debug never reaches a pilot's log", got)
	}
	if got := records[0]["adapter"]; got != "X-Plane" {
		t.Errorf("adapter = %v, want X-Plane", got)
	}
}

// The auto-connect loop retries every 30 seconds for as long as the app is
// open, so this cannot be one line per attempt.
func TestSimulatorSilenceIsRateLimited(t *testing.T) {
	logs := captureLogs(t)
	a := &App{}

	for i := 0; i < 30; i++ {
		a.noteSimWaitFailed("X-Plane")
	}

	// The first, then every tenth: 1, 10, 20, 30.
	if got := len(logs()); got != 4 {
		t.Errorf("30 failed attempts logged %d lines, want 4: %v", got, messages(logs()))
	}
}

// Once the simulator does start sending, the count resets so the next outage
// is reported from the beginning rather than swallowed by the old streak.
func TestSimulatorSilenceCountResets(t *testing.T) {
	a := &App{}
	for i := 0; i < 5; i++ {
		a.noteSimWaitFailed("X-Plane")
	}

	a.simMu.Lock()
	a.simWaitFailures = 0
	a.simMu.Unlock()

	logs := captureLogs(t)
	a.noteSimWaitFailed("X-Plane")

	if got := len(logs()); got != 1 {
		t.Errorf("the first failure after a recovery logged %d lines, want 1", got)
	}
}

// Signing out is something the app does to the pilot, so it says so and says
// which tenant rejected the token. Without this, "I got kicked back to the
// airline list" leaves nothing in the log to find.
func TestSigningOutSaysWhy(t *testing.T) {
	logs := captureLogs(t)

	api := &unauthorizedCapturingAPI{}
	ui := &noopUI{}
	NewApp(api, ui, noopStorage{}, noopDiscord{}, nil, nil)

	if api.onUnauth == nil {
		t.Fatal("NewApp did not register an unauthorized handler")
	}
	api.onUnauth()

	var found map[string]any
	for _, r := range logs() {
		if strings.Contains(r["msg"].(string), "signing out") {
			found = r
		}
	}
	if found == nil {
		t.Fatalf("signing out was not logged: %v", messages(logs()))
	}
	if got := found["level"]; got != "WARN" {
		t.Errorf("level = %v, want WARN", got)
	}
}
