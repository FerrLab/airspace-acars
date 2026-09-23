package app

import (
	"database/sql"
	"testing"

	"airspace-acars/internal/domain"
)

// unauthorizedCapturingAPI is a stubAPI that remembers the callback NewApp
// registers via OnUnauthorized, so a test can trigger it the way a live 401
// would and check what NewApp wired it to do.
type unauthorizedCapturingAPI struct {
	stubAPI
	token      string
	onUnauth   func()
	tokenCalls int
}

func (a *unauthorizedCapturingAPI) SetToken(token string) {
	a.token = token
	a.tokenCalls++
}

func (a *unauthorizedCapturingAPI) OnUnauthorized(fn func()) { a.onUnauth = fn }

// noopUI records every event it is asked to emit.
type noopUI struct {
	events []string
}

func (u *noopUI) EmitEvent(name string, data interface{}) { u.events = append(u.events, name) }

// noopStorage satisfies Storage without persisting anything; NewApp's wiring
// under test never touches it.
type noopStorage struct{}

func (noopStorage) SaveFlightData(*domain.FlightData) error { return nil }
func (noopStorage) QueryFlightData() (*sql.Rows, error)     { return nil, nil }
func (noopStorage) PurgeFlightData() error                  { return nil }
func (noopStorage) EnqueuePosition(string, []byte) error    { return nil }
func (noopStorage) PeekOutboxBatch(string, int) ([]int64, [][]byte, error) {
	return nil, nil, nil
}
func (noopStorage) DeleteOutboxBatch([]int64) error { return nil }
func (noopStorage) CountOutbox(string) (int, error) { return 0, nil }

// noopDiscord satisfies DiscordPresence without an IPC connection.
type noopDiscord struct{}

func (noopDiscord) Connect() error                           { return nil }
func (noopDiscord) Disconnect()                              {}
func (noopDiscord) IsConnected() bool                        { return false }
func (noopDiscord) SetActivity(map[string]interface{}) error { return nil }

// A token the server has revoked or let expire otherwise fails the same way
// on every subsequent poll forever, since nothing tells the frontend it is no
// longer signed in. NewApp is expected to wire the adapter's 401 callback to
// clear the stored token and notify the frontend, turning that into a single
// prompt to sign in again.
func TestNewAppSignsOutOnUnauthorized(t *testing.T) {
	api := &unauthorizedCapturingAPI{}
	ui := &noopUI{}

	NewApp(api, ui, noopStorage{}, noopDiscord{}, nil, nil)

	if api.onUnauth == nil {
		t.Fatal("NewApp did not register an OnUnauthorized callback")
	}

	api.token = "stale-token"
	api.onUnauth()

	if api.token != "" {
		t.Errorf("token = %q after a 401, want cleared", api.token)
	}
	if len(ui.events) != 1 || ui.events[0] != "session-expired" {
		t.Errorf("emitted events = %v, want [session-expired]", ui.events)
	}
}
