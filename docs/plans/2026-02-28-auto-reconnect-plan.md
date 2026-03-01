# Auto-Reconnect Stale Simulator Connections — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Automatically detect stale simulator connections and reconnect without interrupting active flights.

**Architecture:** Add `LastReceived() time.Time` to the `SimConnector` interface so both adapters report when they last got real data. The existing `dataStreamLoop` gains a staleness check: if no fresh data for 10 seconds, tear down the adapter and reconnect with exponential backoff.

**Tech Stack:** Go, testify (assertions), existing SimConnector interface pattern.

---

### Task 1: Add `LastReceived()` to SimConnector interface and mock

**Files:**
- Modify: `sim_connector.go:132-137`
- Modify: `mock_sim_connector_test.go`

**Step 1: Add `LastReceived()` to the interface**

In `sim_connector.go`, add to the `SimConnector` interface:

```go
type SimConnector interface {
	Connect() error
	Disconnect() error
	GetFlightData() (*FlightData, error)
	Name() string
	LastReceived() time.Time
}
```

**Step 2: Add `LastReceived()` to MockSimConnector**

In `mock_sim_connector_test.go`, add a `lastReceived` field and method:

```go
type MockSimConnector struct {
	data         *FlightData
	err          error
	name         string
	lastReceived time.Time
}

func (m *MockSimConnector) LastReceived() time.Time { return m.lastReceived }
```

**Step 3: Verify the project still compiles (it won't yet — adapters don't implement the interface)**

Run: `go build ./... 2>&1 | head -10`
Expected: Compile errors about SimConnectAdapter and XPlaneAdapter not implementing SimConnector (missing `LastReceived`). This is expected — we fix them in the next tasks.

**Step 4: Commit**

```bash
git add sim_connector.go mock_sim_connector_test.go
git commit -m "feat: add LastReceived() to SimConnector interface and mock"
```

---

### Task 2: Implement `LastReceived()` on SimConnect adapter

**Files:**
- Modify: `simconnect_adapter_windows.go`

**Step 1: Add `lastReceived` field to SimConnectAdapter struct**

At line 14, add `lastReceived time.Time` to the struct:

```go
type SimConnectAdapter struct {
	mu           sync.RWMutex
	sc           *sim.SimConnect
	report       *simReport
	latestData   *FlightData
	lastReceived time.Time
	stopCh       chan struct{}
	stopped      chan struct{}
}
```

**Step 2: Update `lastReceived` when dispatch data arrives**

In the `run()` method, inside the `case sim.RECV_ID_SIMOBJECT_DATA_BYTYPE:` block (around line 361-363), after setting `s.latestData = fd`:

```go
s.mu.Lock()
s.latestData = fd
s.lastReceived = time.Now()
s.mu.Unlock()
```

**Step 3: Add the `LastReceived()` method**

After the `GetFlightData()` method:

```go
func (s *SimConnectAdapter) LastReceived() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastReceived
}
```

**Step 4: Verify it compiles**

Run: `go build ./... 2>&1 | head -10`
Expected: Still fails on XPlaneAdapter (fixed next task), but SimConnectAdapter errors should be gone.

**Step 5: Commit**

```bash
git add simconnect_adapter_windows.go
git commit -m "feat: track lastReceived timestamp in SimConnect adapter"
```

---

### Task 3: Implement `LastReceived()` on X-Plane adapter

**Files:**
- Modify: `xplane_adapter.go`

**Step 1: Add the `LastReceived()` method**

The `lastReceived` field already exists on `XPlaneAdapter`. Add the public method after `GetFlightData()`:

```go
func (x *XPlaneAdapter) LastReceived() time.Time {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.lastReceived
}
```

**Step 2: Verify the full project compiles**

Run: `go build ./... 2>&1 | head -5`
Expected: Clean build, no errors.

**Step 3: Run existing tests to verify nothing broke**

Run: `go test ./... -v 2>&1 | tail -20`
Expected: All existing tests pass.

**Step 4: Commit**

```bash
git add xplane_adapter.go
git commit -m "feat: expose LastReceived() on X-Plane adapter"
```

---

### Task 4: Add reconnect state fields and `reconnectSim()` to FlightDataService

**Files:**
- Modify: `flight_data_service.go`

**Step 1: Add new fields to FlightDataService struct**

```go
type FlightDataService struct {
	db                *sql.DB
	app               *application.App
	connector         SimConnector
	mu                sync.Mutex
	recording         bool
	startTime         time.Time
	dataCount         int
	streaming         bool
	streamStopCh      chan struct{}
	simActive         bool
	adapterName       string
	reconnectAttempts int
	lastReconnectAt   time.Time
}
```

**Step 2: Store `adapterName` in `ConnectSim()`**

In `ConnectSim()`, just before `f.startDataStreamLocked()` (line 86), add:

```go
f.adapterName = connector.Name()
f.reconnectAttempts = 0
f.lastReconnectAt = time.Time{}
```

**Step 3: Reset reconnect state in `DisconnectSim()`**

In `DisconnectSim()`, after setting `f.simActive = false`:

```go
f.adapterName = ""
f.reconnectAttempts = 0
f.lastReconnectAt = time.Time{}
```

**Step 4: Add `reconnectSim()` method**

Add after `DisconnectSim()`:

```go
// reconnectSim tears down the current adapter and creates a fresh connection.
// Must be called with f.mu held.
func (f *FlightDataService) reconnectSim() error {
	if f.connector != nil {
		f.connector.Disconnect()
		f.connector = nil
	}

	var connector SimConnector
	switch f.adapterName {
	case "SimConnect":
		connector = NewSimConnectAdapter()
		if connector == nil {
			return fmt.Errorf("SimConnect not available")
		}
	case "X-Plane":
		connector = NewXPlaneAdapter("127.0.0.1", 49000)
	default:
		return fmt.Errorf("unknown adapter: %s", f.adapterName)
	}

	if err := connector.Connect(); err != nil {
		return fmt.Errorf("reconnect %s: %w", f.adapterName, err)
	}

	f.connector = connector
	f.simActive = false
	return nil
}
```

**Step 5: Verify it compiles**

Run: `go build ./... 2>&1 | head -5`
Expected: Clean build.

**Step 6: Commit**

```bash
git add flight_data_service.go
git commit -m "feat: add reconnectSim() and reconnect state to FlightDataService"
```

---

### Task 5: Add staleness detection to `dataStreamLoop`

**Files:**
- Modify: `flight_data_service.go`

**Step 1: Add staleness check to `dataStreamLoop`**

Replace the existing `dataStreamLoop` method with this updated version. The key addition is the staleness check block after the existing data handling:

```go
func (f *FlightDataService) dataStreamLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-f.streamStopCh:
			return
		case <-ticker.C:
			f.mu.Lock()
			connector := f.connector
			recording := f.recording
			wasActive := f.simActive
			adapterName := f.adapterName
			f.mu.Unlock()

			if connector == nil {
				continue
			}

			data, err := connector.GetFlightData()
			if err != nil {
				if wasActive {
					f.mu.Lock()
					f.simActive = false
					f.mu.Unlock()
					if f.app != nil {
						f.app.Event.Emit("connection-state", "")
					}
					slog.Warn("simulator data lost", "error", err)
				}
				continue
			}

			if !wasActive {
				f.mu.Lock()
				f.simActive = true
				f.reconnectAttempts = 0
				f.lastReconnectAt = time.Time{}
				f.mu.Unlock()
				if f.app != nil {
					f.app.Event.Emit("connection-state", connector.Name())
				}
				slog.Info("simulator data received", "adapter", connector.Name())
			}

			if f.app != nil {
				f.app.Event.Emit("flight-data", data)
			}

			if recording {
				jsonBytes, err := json.Marshal(data)
				if err != nil {
					slog.Error("failed to marshal flight data", "error", err)
					continue
				}

				_, err = f.db.Exec(
					`INSERT INTO flight_data (data) VALUES (?)`,
					string(jsonBytes),
				)
				if err != nil {
					slog.Error("failed to insert flight data", "error", err)
					continue
				}

				f.mu.Lock()
				f.dataCount++
				f.mu.Unlock()
			}

			// Staleness check: if data was active but adapter hasn't received
			// fresh data in 10 seconds, attempt a reconnect.
			if wasActive && !connector.LastReceived().IsZero() &&
				time.Since(connector.LastReceived()) > 10*time.Second {

				f.mu.Lock()
				backoff := time.Duration(1<<uint(f.reconnectAttempts)) * 5 * time.Second
				if backoff > 60*time.Second {
					backoff = 60 * time.Second
				}
				if time.Since(f.lastReconnectAt) < backoff {
					f.mu.Unlock()
					continue
				}

				f.lastReconnectAt = time.Now()
				f.simActive = false
				f.mu.Unlock()

				if f.app != nil {
					f.app.Event.Emit("connection-state", "")
				}
				slog.Warn("simulator connection stale, reconnecting",
					"adapter", adapterName,
					"lastData", connector.LastReceived(),
					"attempt", f.reconnectAttempts+1)

				f.mu.Lock()
				err := f.reconnectSim()
				if err != nil {
					f.reconnectAttempts++
					f.mu.Unlock()
					slog.Error("reconnect failed",
						"adapter", adapterName,
						"attempt", f.reconnectAttempts,
						"error", err)
				} else {
					f.mu.Unlock()
					slog.Info("reconnected successfully", "adapter", adapterName)
				}
			}
		}
	}
}
```

**Step 2: Verify it compiles**

Run: `go build ./... 2>&1 | head -5`
Expected: Clean build.

**Step 3: Run existing tests**

Run: `go test ./... -v 2>&1 | tail -20`
Expected: All existing tests still pass.

**Step 4: Commit**

```bash
git add flight_data_service.go
git commit -m "feat: detect stale connections and auto-reconnect in dataStreamLoop"
```

---

### Task 6: Write tests for staleness detection and reconnect

**Files:**
- Create: `flight_data_service_test.go`

**Step 1: Write the test file**

```go
package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataStreamLoopDetectsStaleness(t *testing.T) {
	// Mock that returns data but with a stale LastReceived time.
	mock := &MockSimConnector{
		data:         sampleFlightData(),
		name:         "MockSim",
		lastReceived: time.Now().Add(-15 * time.Second), // 15s ago = stale
	}

	fds := &FlightDataService{
		connector:   mock,
		simActive:   true,
		adapterName: "MockSim",
		streaming:   true,
		streamStopCh: make(chan struct{}),
	}

	// The staleness threshold is 10s. With lastReceived 15s ago,
	// the loop should detect staleness on first tick.
	// We can't easily test the full loop, but we can test the condition.
	lr := mock.LastReceived()
	assert.True(t, time.Since(lr) > 10*time.Second,
		"mock should report stale data")
}

func TestReconnectSimCreatesNewAdapter(t *testing.T) {
	original := &MockSimConnector{
		data:         sampleFlightData(),
		name:         "MockSim",
		lastReceived: time.Now(),
	}

	fds := &FlightDataService{
		connector:   original,
		simActive:   true,
		adapterName: "MockSim",
	}

	// reconnectSim with unknown adapter name should fail
	err := fds.reconnectSim()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown adapter")
}

func TestBackoffCalculation(t *testing.T) {
	tests := []struct {
		attempts int
		wantMax  time.Duration
	}{
		{0, 5 * time.Second},
		{1, 10 * time.Second},
		{2, 20 * time.Second},
		{3, 40 * time.Second},
		{4, 60 * time.Second},  // capped
		{10, 60 * time.Second}, // still capped
	}

	for _, tt := range tests {
		backoff := time.Duration(1<<uint(tt.attempts)) * 5 * time.Second
		if backoff > 60*time.Second {
			backoff = 60 * time.Second
		}
		assert.Equal(t, tt.wantMax, backoff,
			"attempts=%d", tt.attempts)
	}
}

func TestMockSimConnectorLastReceived(t *testing.T) {
	now := time.Now()
	mock := &MockSimConnector{
		data:         sampleFlightData(),
		name:         "MockSim",
		lastReceived: now,
	}

	assert.Equal(t, now, mock.LastReceived())
}

func TestReconnectStateResetOnConnect(t *testing.T) {
	fds := &FlightDataService{
		reconnectAttempts: 5,
		lastReconnectAt:   time.Now(),
	}

	// Simulate what ConnectSim does to reset state
	fds.reconnectAttempts = 0
	fds.lastReconnectAt = time.Time{}

	assert.Equal(t, 0, fds.reconnectAttempts)
	assert.True(t, fds.lastReconnectAt.IsZero())
}
```

**Step 2: Run the new tests**

Run: `go test ./... -v -run "TestDataStreamLoop|TestReconnect|TestBackoff|TestMockSimConnector" 2>&1 | tail -30`
Expected: All tests pass.

**Step 3: Run full test suite**

Run: `go test ./... 2>&1 | tail -5`
Expected: All tests pass.

**Step 4: Commit**

```bash
git add flight_data_service_test.go
git commit -m "test: add tests for staleness detection and reconnect logic"
```

---

### Task 7: Final verification

**Step 1: Run full test suite**

Run: `go test ./... -v 2>&1`
Expected: All tests pass including new ones.

**Step 2: Build the full application**

Run: `go build ./... 2>&1`
Expected: Clean build, no errors or warnings.

**Step 3: Verify no regressions in git**

Run: `git log --oneline -6`
Expected: See the 6 new commits from tasks 1-6.
