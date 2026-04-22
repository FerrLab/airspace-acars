package app

import (
	"database/sql"
	"net/http"
	"sync"
	"time"

	"airspace-acars/internal/domain"

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
	return &App{
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
}
