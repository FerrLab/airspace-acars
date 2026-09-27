package app

import (
	"database/sql"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/internal/profiles"

	"github.com/creativeprojects/go-selfupdate"
)

// --- Adapter interfaces (driven ports) ---

// AirspaceAPI handles all HTTP communication with the Airspace tenant API.
type AirspaceAPI interface {
	SetBaseURL(url string)
	SetToken(token string)
	BaseURL() string
	Token() string
	DoRequest(method, path string, body interface{}) ([]byte, int, error)
	// RawGet performs an unauthenticated GET to an absolute URL.
	RawGet(url string) ([]byte, error)
	// OnUnauthorized registers a callback fired when a request is rejected
	// with a 401, so the app can react to a token going bad mid-session.
	OnUnauthorized(fn func())
}

// UIEmitter sends events to the frontend (Wails event bridge).
type UIEmitter interface {
	EmitEvent(name string, data interface{})
}

// DiscordPresence handles Discord Rich Presence IPC.
type DiscordPresence interface {
	Connect() error
	Disconnect()
	IsConnected() bool
	SetActivity(activity map[string]interface{}) error
}

// Storage handles persistent data storage.
type Storage interface {
	SaveFlightData(data *domain.FlightData) error
	QueryFlightData() (*sql.Rows, error)
	PurgeFlightData() error

	// Position outbox
	EnqueuePosition(bookingID string, payload []byte) error
	PeekOutboxBatch(bookingID string, limit int) ([]int64, [][]byte, error)
	DeleteOutboxBatch(ids []int64) error
	CountOutbox(bookingID string) (int, error)
}

// --- App struct ---

// App is the application layer that orchestrates all business logic.
// Adapters are injected as interfaces; methods are organized as commands and queries.
type App struct {
	// Adapters (driven ports)
	Airspace AirspaceAPI
	UI       UIEmitter
	DB       Storage
	Discord  DiscordPresence

	// Sim connection state
	simMu             sync.Mutex
	connector         domain.SimConnector
	simActive         bool
	adapterName       string
	streaming         bool
	streamStopCh      chan struct{}
	reconnectAttempts int
	lastReconnectAt   time.Time
	simWaitFailures   int
	userDisconnected  bool

	// Recording state
	recording    bool
	recStartTime time.Time
	dataCount    int

	// Flight state
	flightMu       sync.Mutex
	state          string // "idle" | "active" | "finishing"
	callsign       string
	departure      string
	arrival        string
	bookingID      string
	startTime      time.Time
	stopCh         chan struct{}
	finishCancelCh chan struct{}

	// Aircraft profiles
	profileMu        sync.RWMutex
	profileRegistry  *profiles.Registry
	profilesDir      string
	activePlan       *profiles.Plan
	activeProfileSig string

	// Settings
	settingsMu   sync.RWMutex
	settings     domain.Settings
	settingsPath string

	// Auth state
	authMu        sync.RWMutex
	httpClient    *http.Client
	tenantBaseURL string
	token         string

	// Audio
	audioCacheDir string
	audioMu       sync.Mutex
	audioClient   *http.Client

	// Update
	updateLatest *selfupdate.Release

	// Sim connector factories (injected from main for platform-specific construction)
	NewSimConnectAdapter func() domain.SimConnector
	NewXPlaneAdapter     func(host string, port int) domain.SimConnector

	// Auto-flight detection state (only accessed from dataStreamLoop goroutine)
	autoStartArmed bool

	// Discord loop control
	discordNudge chan struct{}

	// Quit callback (for auto-update restart)
	QuitFunc func()
}

// NewApp creates a new App instance with all adapter dependencies.
func NewApp(
	airspace AirspaceAPI,
	ui UIEmitter,
	db Storage,
	discord DiscordPresence,
	newSimConnect func() domain.SimConnector,
	newXPlane func(host string, port int) domain.SimConnector,
) *App {
	a := &App{
		Airspace:             airspace,
		UI:                   ui,
		DB:                   db,
		Discord:              discord,
		NewSimConnectAdapter: newSimConnect,
		NewXPlaneAdapter:     newXPlane,
		state:                "idle",
		httpClient:           &http.Client{Timeout: 30 * time.Second},
		audioClient:          &http.Client{Timeout: 15 * time.Second},
		discordNudge:         make(chan struct{}, 1),
	}

	// A token the server has revoked or let expire fails the same way on
	// every subsequent poll, and the frontend has no way to tell that apart
	// from "still signed in" — it keeps the token and keeps retrying with it,
	// forever, each attempt its own reportable error. Clearing the token here
	// and telling the frontend turns that into a single, one-time prompt to
	// sign in again instead of an unbounded stream of 401s.
	airspace.OnUnauthorized(func() {
		// This is the one place the app signs a pilot out on its own, so it
		// says so. A report of being dropped back to the tenant list is
		// otherwise indistinguishable in the log from the app simply starting.
		slog.Warn("auth: the server rejected our token; signing out",
			"tenant", airspace.BaseURL())
		a.SetToken("")
		a.UI.EmitEvent("session-expired", true)
	})

	return a
}
