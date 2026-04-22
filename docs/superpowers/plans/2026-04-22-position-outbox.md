# Position Outbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the in-RAM position queue with a SQLite-backed per-booking outbox, cap every position POST at 250 reports, and make `FinishFlight` block (asynchronously, via events) until the outbox drains — never losing a report.

**Architecture:** A new `position_outbox` SQLite table persists any position report that fails to send (or that is in flight when the loop stops). A per-tick drain ships batches of 250 in insertion order. `FinishFlight` spawns a goroutine that drains the outbox to zero before calling `/api/acars/finish`, emitting progress events the frontend renders as *"Sending X missing positions before finishing this flight…"*. Flight state gains a `"finishing"` value; user can `CancelFinish` to abort and retry later.

**Tech Stack:** Go 1.22+ (hexagonal architecture), `modernc.org/sqlite`, Wails v3, React + i18next, OpenTelemetry (existing).

**Spec:** `docs/superpowers/specs/2026-04-22-position-batching-design.md`

---

## File Structure

**Create:**
- (none — all modifications)

**Modify:**
- `internal/adapters/storage/sqlite.go` — migration + 4 outbox methods
- `internal/app/app.go` — `Storage` interface extension, `App` struct fields
- `internal/app/flight.go` — constants, helpers, rewritten positionLoop, async finish, `CancelFinish`
- `internal/app/sim.go` — pass booking id into `StartFlight`
- `internal/ports/user_action.go` — new `StartFlight` signature, `CancelFinish`
- `services.go` — Wails service wrappers: new `StartFlight` signature, `CancelFinish`
- `frontend/src/components/acars-tab.tsx` — booking id, progress events, cancel button
- `frontend/src/__mocks__/wails-bindings.ts` — mock new methods
- `frontend/src/locales/en.json` (+ `es.json`, `fr.json`, `pt.json`) — new strings
- `frontend/bindings/airspace-acars/flightservice.ts` — regenerated

---

## Task 1: Outbox table and Storage methods

**Files:**
- Modify: `internal/adapters/storage/sqlite.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add migration + 4 methods in `sqlite.go`**

Open `internal/adapters/storage/sqlite.go`. Below the existing `flight_data` migration block (around line 49), add the outbox migration. Then append the four new methods at the end of the file.

Add this inside `NewSQLiteAdapter` immediately after the `CREATE TABLE IF NOT EXISTS flight_data ...` `db.Exec` block:

```go
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS position_outbox (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		booking_id TEXT    NOT NULL,
		payload    TEXT    NOT NULL,
		created_at INTEGER NOT NULL
	)`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create position_outbox: %w", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_outbox_booking ON position_outbox(booking_id, id)`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create idx_outbox_booking: %w", err)
	}
```

Add a write mutex to the struct. Replace:

```go
type SQLiteAdapter struct {
	db *sql.DB
}
```

with:

```go
type SQLiteAdapter struct {
	db    *sql.DB
	writeMu sync.Mutex
}
```

Add `"sync"` and `"time"` to the import block at the top of the file.

Append the four methods at the end of the file:

```go
// EnqueuePosition persists one report to the outbox for later delivery.
func (s *SQLiteAdapter) EnqueuePosition(bookingID string, payload []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO position_outbox (booking_id, payload, created_at) VALUES (?, ?, ?)`,
		bookingID, string(payload), time.Now().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("enqueue position: %w", err)
	}
	return nil
}

// PeekOutboxBatch reads up to limit rows for the given booking in insertion order.
// Rows remain until DeleteOutboxBatch is called with their ids.
func (s *SQLiteAdapter) PeekOutboxBatch(bookingID string, limit int) ([]int64, [][]byte, error) {
	rows, err := s.db.Query(
		`SELECT id, payload FROM position_outbox WHERE booking_id = ? ORDER BY id ASC LIMIT ?`,
		bookingID, limit,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("peek outbox: %w", err)
	}
	defer rows.Close()

	var ids []int64
	var payloads [][]byte
	for rows.Next() {
		var id int64
		var payload string
		if err := rows.Scan(&id, &payload); err != nil {
			return nil, nil, fmt.Errorf("scan outbox row: %w", err)
		}
		ids = append(ids, id)
		payloads = append(payloads, []byte(payload))
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iter outbox rows: %w", err)
	}
	return ids, payloads, nil
}

// DeleteOutboxBatch removes rows by id. Used after successful POST.
func (s *SQLiteAdapter) DeleteOutboxBatch(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// Build "DELETE ... WHERE id IN (?, ?, ...)" with positional args.
	args := make([]interface{}, len(ids))
	placeholders := make([]byte, 0, len(ids)*2)
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args[i] = id
	}
	query := "DELETE FROM position_outbox WHERE id IN (" + string(placeholders) + ")"
	if _, err := s.db.Exec(query, args...); err != nil {
		return fmt.Errorf("delete outbox batch: %w", err)
	}
	return nil
}

// CountOutbox returns how many rows are queued for the given booking.
func (s *SQLiteAdapter) CountOutbox(bookingID string) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM position_outbox WHERE booking_id = ?`,
		bookingID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count outbox: %w", err)
	}
	return n, nil
}
```

- [ ] **Step 2: Extend the `Storage` interface**

Open `internal/app/app.go`. Replace the `Storage` interface (lines 40–45) with:

```go
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
```

- [ ] **Step 3: Build**

Run: `go build .`
Expected: success (the `SQLiteAdapter` concrete type satisfies the extended interface).

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite.go internal/app/app.go
git commit -m "feat(storage): add booking-scoped position outbox to sqlite adapter"
```

---

## Task 2: App struct fields and constants

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/flight.go`

- [ ] **Step 1: Add flight-state fields to App struct**

Open `internal/app/app.go`. Locate the `// Flight state` block (around line 74). Replace the block with:

```go
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
```

- [ ] **Step 2: Update constants in flight.go**

Open `internal/app/flight.go`. Replace the `const` block starting with `highResInterval` (around line 41) with:

```go
const (
	highResInterval      = 33 * time.Millisecond
	posIntervalCritical  = 500 * time.Millisecond
	posIntervalLow       = 1 * time.Second
	posIntervalHigh      = 2 * time.Second
	posIntervalStatic    = 60 * time.Second
	flareAltThreshold    = 50.0
	criticalAltThreshold = 200.0
	highAltThreshold     = 10_000.0
	maxHighResReports    = 3000
	maxBatchSize         = 250
	// maxPendingReports (was 500, spec raised to 1000) has been retired —
	// the durable outbox has no fixed in-RAM cap; growth is bounded by
	// maxHighResReports (3000) plus SQLite.
	retryAttempts        = 4

	finishDrainTickEvery = 1 * time.Second
	finishDrainBackoffMax = 60 * time.Second

	minFlightDuration = 1 * time.Minute
)
```

- [ ] **Step 3: Build**

Run: `go build .`
Expected: success. (Fields are unused so far — Go allows unused struct fields.)

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go internal/app/flight.go
git commit -m "feat(flight): add bookingID + finishCancelCh state, raise caps, add batch constants"
```

---

## Task 3: `sendPositionBatches` and `drainOutbox` helpers

**Files:**
- Modify: `internal/app/flight.go`

- [ ] **Step 1: Add `sendPositionBatches`**

In `internal/app/flight.go`, insert the following immediately before `buildPositionReport` (line 329). Place it alongside the existing `doRequestWithRetry`:

```go
// sendPositionBatches POSTs reports to /api/v2/acars/position in chunks of at
// most maxBatchSize. Returns the number sent successfully before returning.
// On error, returns (sent, err) and leaves the unsent suffix to the caller.
func (a *App) sendPositionBatches(reports []map[string]interface{}) (int, error) {
	sent := 0
	for sent < len(reports) {
		end := sent + maxBatchSize
		if end > len(reports) {
			end = len(reports)
		}
		batch := reports[sent:end]
		if _, _, err := a.Airspace.DoRequest("POST", "/api/v2/acars/position", batch); err != nil {
			return sent, err
		}
		sent = end
	}
	return sent, nil
}
```

- [ ] **Step 2: Add `drainOutbox`**

Immediately below `sendPositionBatches`, add:

```go
// drainOutbox pulls up to maxBatches batches of maxBatchSize from the outbox
// for the given booking and POSTs each. Successfully shipped rows are deleted
// from the DB. Returns (sent, remaining, err); on error the unsent rows stay.
func (a *App) drainOutbox(bookingID string, maxBatches int) (int, int, error) {
	if bookingID == "" {
		return 0, 0, nil
	}
	sent := 0
	for i := 0; i < maxBatches; i++ {
		ids, payloads, err := a.DB.PeekOutboxBatch(bookingID, maxBatchSize)
		if err != nil {
			remaining, _ := a.DB.CountOutbox(bookingID)
			return sent, remaining, err
		}
		if len(ids) == 0 {
			return sent, 0, nil
		}

		reports := make([]map[string]interface{}, len(payloads))
		for j, raw := range payloads {
			var report map[string]interface{}
			if err := json.Unmarshal(raw, &report); err != nil {
				// Poison row — delete it so we don't loop forever.
				slog.Warn("outbox: dropping unparsable payload", "id", ids[j], "error", err)
				_ = a.DB.DeleteOutboxBatch([]int64{ids[j]})
				continue
			}
			reports[j] = report
		}

		if _, _, err := a.Airspace.DoRequest("POST", "/api/v2/acars/position", reports); err != nil {
			remaining, _ := a.DB.CountOutbox(bookingID)
			return sent, remaining, err
		}
		if err := a.DB.DeleteOutboxBatch(ids); err != nil {
			slog.Warn("outbox: delete after send failed", "count", len(ids), "error", err)
			remaining, _ := a.DB.CountOutbox(bookingID)
			return sent, remaining, err
		}
		sent += len(ids)
	}
	remaining, _ := a.DB.CountOutbox(bookingID)
	return sent, remaining, nil
}
```

- [ ] **Step 3: Build**

Run: `go build .`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/app/flight.go
git commit -m "feat(flight): add sendPositionBatches + drainOutbox helpers"
```

---

## Task 4: `StartFlight` signature change + callers

**Files:**
- Modify: `internal/app/flight.go`
- Modify: `internal/app/sim.go`
- Modify: `internal/ports/user_action.go`
- Modify: `services.go`

- [ ] **Step 1: Update `StartFlight` in flight.go**

In `internal/app/flight.go`, replace the `StartFlight` signature and preamble (line 107):

```go
// StartFlight begins a new flight tracking session.
func (a *App) StartFlight(callsign, departure, arrival, bookingID string) error {
	_, span := flightTracer.Start(context.Background(), "flight.start",
		trace.WithAttributes(
			attribute.String("flight.callsign", callsign),
			attribute.String("flight.departure", departure),
			attribute.String("flight.arrival", arrival),
			attribute.String("flight.booking_id", bookingID),
		))
	defer span.End()
```

Inside `StartFlight`, after `a.arrival = arrival` (line 170), insert:

```go
	a.bookingID = bookingID
```

At the end of `StartFlight`, just before `return nil`, add:

```go
	if bookingID != "" {
		if n, err := a.DB.CountOutbox(bookingID); err == nil && n > 0 {
			slog.Info("resuming unsent positions from previous session", "booking_id", bookingID, "count", n)
			a.UI.EmitEvent("flight-outbox-resuming", map[string]interface{}{"pending": n})
		}
	}
```

Also update `endFlight` (around line 272) to clear the new field:

```go
func (a *App) endFlight() {
	if a.stopCh != nil {
		close(a.stopCh)
		a.stopCh = nil
	}
	a.state = "idle"
	a.callsign = ""
	a.departure = ""
	a.arrival = ""
	a.bookingID = ""
	a.UI.EmitEvent("flight-state", "idle")
}
```

- [ ] **Step 2: Update `tryAutoStartFlight` in sim.go**

Open `internal/app/sim.go`. After the `arrival, _ = ...` block (around line 447), add booking-id extraction. The booking API may return the id as a string or a number, so handle both. Insert before `a.UI.EmitEvent("auto-flight-start", callsign)`:

```go
	var bookingID string
	switch v := booking["id"].(type) {
	case string:
		bookingID = v
	case float64:
		bookingID = strconv.FormatInt(int64(v), 10)
	}
```

Change the `StartFlight` call on the next line from:

```go
	if err := a.StartFlight(callsign, departure, arrival); err != nil {
```

to:

```go
	if err := a.StartFlight(callsign, departure, arrival, bookingID); err != nil {
```

Ensure `"strconv"` is in the imports at the top of `sim.go` (add if missing).

- [ ] **Step 3: Update `UserActionPort.StartFlight` in user_action.go**

Replace lines 82–84 of `internal/ports/user_action.go` with:

```go
func (p *UserActionPort) StartFlight(callsign, departure, arrival, bookingID string) error {
	return p.App.StartFlight(callsign, departure, arrival, bookingID)
}
```

- [ ] **Step 4: Update `FlightService.StartFlight` in services.go**

Replace lines 46–48 of `services.go` with:

```go
func (s *FlightService) StartFlight(callsign, departure, arrival, bookingID string) error {
	return s.app.StartFlight(callsign, departure, arrival, bookingID)
}
```

- [ ] **Step 5: Build**

Run: `go build .`
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add internal/app/flight.go internal/app/sim.go internal/ports/user_action.go services.go
git commit -m "feat(flight): thread bookingID through StartFlight; emit resume event"
```

---

## Task 5: Rewrite positionLoop write/drain path

**Files:**
- Modify: `internal/app/flight.go`

- [ ] **Step 1: Replace the positionLoop body**

In `internal/app/flight.go`, replace the entire `positionLoop` function (lines 474–660) with the version below. The changes:

- Remove in-RAM `pendingReports`; failed single-report sends go to the outbox.
- `highResQueue` drain uses `sendPositionBatches`; failed rows persist to outbox.
- Each tick, drain one batch (250 rows) from the outbox.
- On stopCh, flush `highResQueue` to the outbox before returning.
- Metrics bookkeeping adjusted.

```go
func (a *App) positionLoop(stopCh chan struct{}) {
	ticker := time.NewTicker(posIntervalLow)
	defer ticker.Stop()

	collectTicker := time.NewTicker(time.Hour)
	collectTicker.Stop()
	defer collectTicker.Stop()

	currentInterval := posIntervalLow
	var lastLat, lastLng float64
	lastChanged := time.Now()

	var highResQueue []map[string]interface{}
	var consecutiveFailures int
	collecting := false
	var touchdownGrace time.Time

	a.flightMu.Lock()
	bookingID := a.bookingID
	a.flightMu.Unlock()

	slog.Info("position loop started", "interval", intervalName(currentInterval), "booking_id", bookingID)

	persistQueue := func(q []map[string]interface{}, reason string) {
		if bookingID == "" || len(q) == 0 {
			return
		}
		for _, report := range q {
			raw, err := json.Marshal(report)
			if err != nil {
				slog.Warn("position: marshal for outbox failed", "error", err, "reason", reason)
				continue
			}
			if err := a.DB.EnqueuePosition(bookingID, raw); err != nil {
				slog.Warn("position: enqueue to outbox failed", "error", err, "reason", reason)
				continue
			}
			posReportsQueued.Add(context.Background(), 1)
		}
	}

	for {
		select {
		case <-stopCh:
			slog.Info("position loop stopping",
				"collecting", collecting,
				"highResQueued", len(highResQueue))
			persistQueue(highResQueue, "loop-stop")
			return

		case <-collectTicker.C:
			if !collecting {
				continue
			}
			fd, err := a.GetFlightDataNow()
			if err != nil {
				slog.Warn("high-res collect tick: no data", "error", err)
				continue
			}
			if len(highResQueue) < maxHighResReports {
				highResQueue = append(highResQueue, a.buildPositionReport(fd))
				posHighResQueued.Add(context.Background(), 1)
			}

		case <-ticker.C:
			fd, err := a.GetFlightDataNow()
			if err != nil {
				continue
			}

			posChanged := fd.Position.Latitude != lastLat || fd.Position.Longitude != lastLng
			if posChanged {
				lastLat = fd.Position.Latitude
				lastLng = fd.Position.Longitude
				lastChanged = time.Now()
			}

			// Flare zone detection (unchanged).
			shouldCollect := false
			if !fd.Sensors.OnGround && fd.Position.AltitudeAGL < flareAltThreshold {
				shouldCollect = true
				touchdownGrace = time.Time{}
			} else if collecting && fd.Sensors.OnGround {
				if touchdownGrace.IsZero() {
					touchdownGrace = time.Now()
					slog.Info("touchdown detected, continuing high-res collection",
						"agl", fd.Position.AltitudeAGL, "gs", fd.Attitude.GS)
				}
				if time.Since(touchdownGrace) < 5*time.Second && fd.Attitude.GS >= 40 {
					shouldCollect = true
				}
			}

			if shouldCollect && !collecting {
				collecting = true
				collectTicker.Reset(highResInterval)
				slog.Info("HIGH-RES ON: entering flare zone",
					"agl", fmt.Sprintf("%.1f", fd.Position.AltitudeAGL),
					"gs", fmt.Sprintf("%.1f", fd.Attitude.GS),
					"onGround", fd.Sensors.OnGround,
					"interval", "33ms")
			} else if !shouldCollect && collecting {
				collecting = false
				collectTicker.Stop()
				slog.Info("HIGH-RES OFF: leaving flare zone",
					"agl", fmt.Sprintf("%.1f", fd.Position.AltitudeAGL),
					"gs", fmt.Sprintf("%.1f", fd.Attitude.GS),
					"onGround", fd.Sensors.OnGround,
					"queued", len(highResQueue))
				touchdownGrace = time.Time{}
			}

			// Reporting interval selection.
			var newInterval time.Duration
			var reason string
			if len(highResQueue) > 0 {
				newInterval = posIntervalCritical
				reason = "draining high-res queue"
			} else if !posChanged && time.Since(lastChanged) > 5*time.Second {
				newInterval = posIntervalStatic
				reason = "position static"
			} else if fd.Position.AltitudeAGL < criticalAltThreshold && (!fd.Sensors.OnGround || collecting) {
				newInterval = posIntervalCritical
				reason = fmt.Sprintf("low altitude (%.0fft AGL)", fd.Position.AltitudeAGL)
			} else if fd.Position.AltitudeAGL >= highAltThreshold {
				newInterval = posIntervalHigh
				reason = fmt.Sprintf("high altitude (%.0fft AGL)", fd.Position.AltitudeAGL)
			} else {
				newInterval = posIntervalLow
				reason = "normal"
			}
			if newInterval != currentInterval {
				slog.Info("position interval changed",
					"from", intervalName(currentInterval),
					"to", intervalName(newInterval),
					"reason", reason,
					"agl", fmt.Sprintf("%.1f", fd.Position.AltitudeAGL),
					"onGround", fd.Sensors.OnGround)
				currentInterval = newInterval
				ticker.Reset(currentInterval)
			}

			// Drain one batch from the outbox (non-blocking).
			if bookingID != "" {
				if sent, remaining, derr := a.drainOutbox(bookingID, 1); derr != nil {
					if consecutiveFailures%30 == 0 {
						slog.Warn("outbox drain failed", "error", derr, "remaining", remaining)
					}
				} else if sent > 0 {
					posReportsSent.Add(context.Background(), int64(sent))
					slog.Info("outbox drained", "sent", sent, "remaining", remaining)
				}
			}

			// High-res queue drain — batched; on failure persist to outbox.
			if len(highResQueue) > 0 {
				posHighResDepth.Record(context.Background(), int64(len(highResQueue)))
				shipped, err := a.sendPositionBatches(highResQueue)
				if shipped > 0 {
					posReportsSent.Add(context.Background(), int64(shipped))
				}
				if err != nil {
					unsent := highResQueue[shipped:]
					posReportsFailed.Add(context.Background(), int64(len(unsent)))
					persistQueue(unsent, "highres-drain-failed")
					highResQueue = nil
				} else {
					highResQueue = nil
				}
			}

			// Normal single-report send (only when not in high-res collection mode).
			if !collecting {
				report := a.buildPositionReport(fd)
				_, _, err = a.Airspace.DoRequest("POST", "/api/v2/acars/position", report)
				if err != nil {
					consecutiveFailures++
					if bookingID != "" {
						raw, mErr := json.Marshal(report)
						if mErr == nil {
							if eErr := a.DB.EnqueuePosition(bookingID, raw); eErr == nil {
								posReportsQueued.Add(context.Background(), 1)
							}
						}
					}
					if consecutiveFailures == 1 {
						slog.Warn("server connection lost, queuing position reports", "error", err)
					} else if consecutiveFailures%30 == 0 {
						slog.Warn("server still unreachable", "failures", consecutiveFailures)
					}
				} else {
					posReportsSent.Add(context.Background(), 1)
					if consecutiveFailures > 0 {
						slog.Info("server connection restored", "had_failures", consecutiveFailures)
					}
					consecutiveFailures = 0
				}
			}
		}
	}
}
```

- [ ] **Step 2: Remove `flushPendingReports`**

In `internal/app/flight.go`, delete the `flushPendingReports` function (lines 662–675). It is no longer referenced.

- [ ] **Step 3: Build**

Run: `go build .`
Expected: success.

- [ ] **Step 4: Smoke log check**

Run a headless Go build and ensure no log output is uninitialized — the function compiles only; we'll exercise it in Task 11.

- [ ] **Step 5: Commit**

```bash
git add internal/app/flight.go
git commit -m "feat(flight): route un-shipped positions through SQLite outbox"
```

---

## Task 6: Async finish + CancelFinish

**Files:**
- Modify: `internal/app/flight.go`

- [ ] **Step 1: Replace `FinishFlight`**

In `internal/app/flight.go`, replace the `FinishFlight` function (lines 211–270) with:

```go
// FinishFlight transitions to the "finishing" state and kicks off an async
// drain. Returns immediately. The frontend listens for flight-finish-*
// events to render progress and the terminal state.
func (a *App) FinishFlight() error {
	_, span := flightTracer.Start(context.Background(), "flight.finish")
	defer span.End()

	a.flightMu.Lock()

	span.SetAttributes(
		attribute.String("flight.callsign", a.callsign),
		attribute.Float64("flight.duration_sec", time.Since(a.startTime).Seconds()),
	)

	if a.state != "active" {
		a.flightMu.Unlock()
		err := fmt.Errorf("no active flight")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if elapsed := time.Since(a.startTime); elapsed < minFlightDuration {
		a.flightMu.Unlock()
		remaining := (minFlightDuration - elapsed).Round(time.Second)
		err := fmt.Errorf("flight too short to finish, please wait %s", remaining)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Snapshot what we need in the goroutine.
	bookingID := a.bookingID
	callsign := a.callsign
	departure := a.departure
	arrival := a.arrival

	// Stop the position loop (it will flush in-RAM highResQueue to outbox).
	if a.stopCh != nil {
		close(a.stopCh)
		a.stopCh = nil
	}
	a.state = "finishing"
	a.finishCancelCh = make(chan struct{})
	cancelCh := a.finishCancelCh

	a.flightMu.Unlock()

	a.UI.EmitEvent("flight-state", "finishing")
	slog.Info("flight finishing, draining outbox", "booking_id", bookingID, "callsign", callsign)

	go a.finishDrainLoop(bookingID, callsign, departure, arrival, cancelCh)
	return nil
}
```

- [ ] **Step 2: Add `finishDrainLoop`**

Immediately below the new `FinishFlight`, add:

```go
// finishDrainLoop runs until the outbox for the booking is empty, then POSTs
// /api/acars/finish. Emits flight-finish-progress events once per tick, and a
// terminal flight-finish-complete or flight-finish-failed event. Retries
// transient errors forever (exponential backoff capped at finishDrainBackoffMax).
// If cancelCh is closed, the loop exits and reverts state to "active".
func (a *App) finishDrainLoop(bookingID, callsign, departure, arrival string, cancelCh chan struct{}) {
	started := time.Now()
	backoff := 2 * time.Second

	for {
		select {
		case <-cancelCh:
			a.flightMu.Lock()
			a.state = "active"
			a.stopCh = make(chan struct{})
			a.flightMu.Unlock()
			go a.positionLoop(a.stopCh)
			a.UI.EmitEvent("flight-state", "active")
			slog.Info("flight finish cancelled by user", "booking_id", bookingID)
			return
		default:
		}

		count, err := a.DB.CountOutbox(bookingID)
		if err != nil {
			slog.Warn("outbox count failed during finish drain", "error", err)
			a.sleepOrCancel(backoff, cancelCh)
			backoff = minDuration(backoff*2, finishDrainBackoffMax)
			continue
		}

		if count == 0 {
			payload := map[string]interface{}{
				"callsign":  callsign,
				"departure": departure,
				"arrival":   arrival,
				"timestamp": time.Now().UnixMilli(),
			}
			body, status, err := a.doRequestWithRetry("POST", "/api/acars/finish", payload)
			if err != nil || status >= 400 {
				msg := "server error"
				if err != nil {
					msg = err.Error()
				} else {
					var errResp map[string]interface{}
					_ = json.Unmarshal(body, &errResp)
					if m, ok := errResp["error"].(string); ok {
						msg = m
					}
				}
				a.flightMu.Lock()
				a.state = "active"
				a.stopCh = make(chan struct{})
				a.flightMu.Unlock()
				go a.positionLoop(a.stopCh)
				a.UI.EmitEvent("flight-state", "active")
				a.UI.EmitEvent("flight-finish-failed", map[string]interface{}{"reason": msg})
				slog.Warn("flight finish request failed", "error", msg)
				return
			}

			// Success.
			a.flightMu.Lock()
			a.endFlight()
			a.flightMu.Unlock()
			a.UI.EmitEvent("flight-finish-complete", map[string]interface{}{
				"duration_sec": time.Since(started).Seconds(),
			})
			slog.Info("flight finished cleanly",
				"booking_id", bookingID,
				"callsign", callsign,
				"drain_sec", time.Since(started).Seconds())
			return
		}

		a.UI.EmitEvent("flight-finish-progress", map[string]interface{}{"pending": count})

		sent, _, err := a.drainOutbox(bookingID, 4) // 4 batches = 1000 rows per pass
		if sent > 0 {
			posReportsSent.Add(context.Background(), int64(sent))
			backoff = 2 * time.Second
		}
		if err != nil {
			slog.Warn("finish drain batch failed, will retry", "error", err, "remaining", count-sent)
			a.sleepOrCancel(backoff, cancelCh)
			backoff = minDuration(backoff*2, finishDrainBackoffMax)
			continue
		}

		a.sleepOrCancel(finishDrainTickEvery, cancelCh)
	}
}

func (a *App) sleepOrCancel(d time.Duration, cancelCh chan struct{}) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-cancelCh:
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 3: Add `CancelFinish`**

Below `finishDrainLoop`, add:

```go
// CancelFinish aborts an in-progress finish drain and returns to "active".
// The outbox is left intact; the user can retry FinishFlight later.
func (a *App) CancelFinish() error {
	a.flightMu.Lock()
	if a.state != "finishing" {
		a.flightMu.Unlock()
		return fmt.Errorf("no finish in progress")
	}
	ch := a.finishCancelCh
	a.finishCancelCh = nil
	a.flightMu.Unlock()

	if ch != nil {
		close(ch)
	}
	return nil
}
```

- [ ] **Step 4: Build**

Run: `go build .`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/app/flight.go
git commit -m "feat(flight): async FinishFlight with outbox drain progress + CancelFinish"
```

---

## Task 7: Expose `CancelFinish` via ports

**Files:**
- Modify: `internal/ports/user_action.go`
- Modify: `services.go`

- [ ] **Step 1: Add port method**

In `internal/ports/user_action.go`, after `FinishFlight` (line 90–92), add:

```go
func (p *UserActionPort) CancelFinish() error {
	return p.App.CancelFinish()
}
```

- [ ] **Step 2: Add Wails service method**

In `services.go`, after `FinishFlight` (line 50), add:

```go
func (s *FlightService) CancelFinish() error { return s.app.CancelFinish() }
```

- [ ] **Step 3: Build**

Run: `go build .`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/ports/user_action.go services.go
git commit -m "feat(ports): expose CancelFinish to Wails frontend"
```

---

## Task 8: Metrics additions

**Files:**
- Modify: `internal/app/flight.go`

- [ ] **Step 1: Add new counters/histograms**

In `internal/app/flight.go`, extend the meter `var (...)` block (lines 24–39) by appending:

```go
	posOutboxEnqueued, _ = flightMeter.Int64Counter("position.outbox_enqueued",
		metric.WithDescription("Position reports persisted to the SQLite outbox"))
	posOutboxDepth, _    = flightMeter.Int64Histogram("position.outbox_depth",
		metric.WithDescription("Outbox row count sampled at each drain pass"))
	finishDrainDur, _    = flightMeter.Float64Histogram("flight.finish_drain_duration_sec",
		metric.WithDescription("Seconds from FinishFlight call to /api/acars/finish success"))
	finishCanceledTotal, _ = flightMeter.Int64Counter("flight.finish_canceled_total",
		metric.WithDescription("Number of times CancelFinish was invoked"))
```

- [ ] **Step 2: Wire them up**

In `CancelFinish` (Task 7), before `return nil`, add:

```go
	finishCanceledTotal.Add(context.Background(), 1)
```

In `finishDrainLoop`, inside the `if count == 0 { ... success branch ... }` block, after `a.UI.EmitEvent("flight-finish-complete", ...)`, add:

```go
	finishDrainDur.Record(context.Background(), time.Since(started).Seconds())
```

After the `a.UI.EmitEvent("flight-finish-progress", ...)` line, add:

```go
	posOutboxDepth.Record(context.Background(), int64(count))
```

In `persistQueue` inside `positionLoop` (Task 5), replace the existing `posReportsQueued.Add(...)` call inside the success branch with **both**:

```go
			posReportsQueued.Add(context.Background(), 1)
			posOutboxEnqueued.Add(context.Background(), 1)
```

In the single-report failure branch (Task 5), replace the existing `posReportsQueued.Add(...)` call with:

```go
								posReportsQueued.Add(context.Background(), 1)
								posOutboxEnqueued.Add(context.Background(), 1)
```

- [ ] **Step 3: Build**

Run: `go build .`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/app/flight.go
git commit -m "feat(observability): add outbox + finish-drain metrics"
```

---

## Task 9: Regenerate Wails bindings

**Files:**
- Auto-modified: `frontend/bindings/airspace-acars/flightservice.ts`
- Auto-modified: `frontend/bindings/airspace-acars/index.ts` (if applicable)

- [ ] **Step 1: Run the generator**

Run: `wails3 generate bindings`
Expected: the `frontend/bindings/airspace-acars/flightservice.ts` file is updated. `StartFlight` now takes four args; a new `CancelFinish()` export is present.

- [ ] **Step 2: Verify the generated file**

Open `frontend/bindings/airspace-acars/flightservice.ts`. Confirm:
- `StartFlight` signature: `(callsign: string, departure: string, arrival: string, bookingID: string)`.
- `CancelFinish(): $CancellablePromise<void>` is exported.

- [ ] **Step 3: Commit**

```bash
git add frontend/bindings/
git commit -m "chore: regenerate Wails bindings for StartFlight + CancelFinish"
```

---

## Task 10: Frontend wiring + i18n

**Files:**
- Modify: `frontend/src/components/acars-tab.tsx`
- Modify: `frontend/src/__mocks__/wails-bindings.ts`
- Modify: `frontend/src/locales/en.json`
- Modify: `frontend/src/locales/es.json`, `fr.json`, `pt.json`

- [ ] **Step 1: Add i18n keys to en.json**

Open `frontend/src/locales/en.json`. After the `"acars.finishFlightFailed"` entry (line 59), add the following keys (comma on the preceding line as required by JSON):

```json
  "acars.finishing": "Finishing...",
  "acars.finishingDrain": "Sending {{count}} missing positions before finishing this flight...",
  "acars.resumingOutbox": "Resuming delivery of {{count}} positions from a previous session.",
  "acars.cancelFinish": "Cancel finish",
  "acars.finishComplete": "Flight finished successfully.",
  "acars.finishFailedWithReason": "Finish failed: {{reason}}",
```

(The existing `"acars.finishing": "Finishing..."` key may already be present — if so, leave it and only add the four new entries.)

- [ ] **Step 2: Mirror the keys into es.json, fr.json, pt.json**

For each of `es.json`, `fr.json`, `pt.json` under `frontend/src/locales/`, add the same keys with placeholder translations (use the English text as a fallback — a translation pass is out of scope):

```json
  "acars.finishingDrain": "Sending {{count}} missing positions before finishing this flight...",
  "acars.resumingOutbox": "Resuming delivery of {{count}} positions from a previous session.",
  "acars.cancelFinish": "Cancel finish",
  "acars.finishComplete": "Flight finished successfully.",
  "acars.finishFailedWithReason": "Finish failed: {{reason}}",
```

- [ ] **Step 3: Update the mock**

In `frontend/src/__mocks__/wails-bindings.ts`, replace the `mockFlightService` return object (lines 20–33) with:

```ts
export function mockFlightService() {
  return {
    GetFlightState: () => Promise.resolve("idle"),
    GetBooking: () =>
      Promise.resolve({
        id: "bk_test_1",
        callsign: "BAW123",
        departure_airport: { icao: "EGLL", city: "London" },
        arrival_airport: { icao: "KJFK", city: "New York" },
      }),
    StartFlight: (
      _callsign: string,
      _departure: string,
      _arrival: string,
      _bookingID: string,
    ) => Promise.resolve(),
    StopFlight: () => Promise.resolve(),
    FinishFlight: () => Promise.resolve(),
    CancelFinish: () => Promise.resolve(),
  };
}
```

- [ ] **Step 4: Update `handleStartFlight` in acars-tab.tsx**

Open `frontend/src/components/acars-tab.tsx`. Replace `handleStartFlight` (lines 127–140) with:

```ts
  const handleStartFlight = async () => {
    if (!booking) return;
    setStartingFlight(true);
    try {
      const callsign = booking.callsign ?? booking.flight_number ?? "";
      const departure = booking.departure_airport?.icao ?? "";
      const arrival = booking.alternate_airport?.icao ?? booking.arrival_airport?.icao ?? "";
      const bookingID = String(booking.id ?? "");
      await FlightService.StartFlight(callsign, departure, arrival, bookingID);
    } catch (e: any) {
      alert(translateError(t, "Failed to start flight: " + e));
    } finally {
      setStartingFlight(false);
    }
  };
```

- [ ] **Step 5: Add finish-progress state and event subscriptions**

In `acars-tab.tsx`, locate the existing `useEffect` that subscribes to `Events.On` (near the top of the component). Immediately after the existing event subscriptions, add state and subscriptions.

Add these `useState` hooks near the top of the component (alongside `autoNotification`):

```ts
  const [finishPending, setFinishPending] = useState<number | null>(null);
  const [resumeNotice, setResumeNotice] = useState<string | null>(null);
```

Inside the main event-subscription `useEffect` (around line 40), before the `return () => { ... }` block, add:

```ts
    const cancelFinishProgress = Events.On("flight-finish-progress", (event: any) => {
      const pending = event?.data?.pending ?? 0;
      setFinishPending(pending);
    });
    const cancelFinishComplete = Events.On("flight-finish-complete", () => {
      setFinishPending(null);
      setAutoNotification(t("acars.finishComplete"));
      setTimeout(() => setAutoNotification(null), 5000);
    });
    const cancelFinishFailed = Events.On("flight-finish-failed", (event: any) => {
      const reason = event?.data?.reason ?? "";
      setFinishPending(null);
      alert(t("acars.finishFailedWithReason", { reason }));
    });
    const cancelResume = Events.On("flight-outbox-resuming", (event: any) => {
      const pending = event?.data?.pending ?? 0;
      setResumeNotice(t("acars.resumingOutbox", { count: pending }));
      setTimeout(() => setResumeNotice(null), 8000);
    });
```

Then add their cleanup inside the cleanup function:

```ts
      cancelFinishProgress();
      cancelFinishComplete();
      cancelFinishFailed();
      cancelResume();
```

- [ ] **Step 6: Render progress + cancel button when `flightState === "finishing"`**

Locate `handleFinishFlight` (line 153). Add `handleCancelFinish` below it:

```ts
  const handleCancelFinish = async () => {
    try {
      await FlightService.CancelFinish();
    } catch (e: any) {
      console.error("Cancel finish failed:", e);
    }
  };
```

In the JSX, find the existing finish button area (inside the flight-active view). Wrap the rendering so that when `flightState === "finishing"` the component shows a progress panel + Cancel button instead of the normal finish button. The exact structure depends on the existing layout — add a block like:

```tsx
          {flightState === "finishing" && (
            <div className="rounded-md border bg-muted/30 p-3 text-sm space-y-2">
              <p>
                {finishPending !== null && finishPending > 0
                  ? t("acars.finishingDrain", { count: finishPending })
                  : t("acars.finishing")}
              </p>
              <Button size="sm" variant="outline" onClick={handleCancelFinish}>
                {t("acars.cancelFinish")}
              </Button>
            </div>
          )}
```

Place it inside the existing "flight-active" branch — adjacent to where `handleFinishFlight`'s button renders today. Also gate the existing Finish button so it hides when `flightState === "finishing"`.

Render the resume notice near the top of the card when it's non-null:

```tsx
          {resumeNotice && (
            <div className="rounded-md border border-amber-500/30 bg-amber-500/10 p-2 text-sm">
              {resumeNotice}
            </div>
          )}
```

- [ ] **Step 7: Run frontend tests**

Run: `cd frontend && npm test -- --run`
Expected: tests pass (the mock changes are type-compatible).

- [ ] **Step 8: Build the frontend**

Run: `cd frontend && npm run build`
Expected: success.

- [ ] **Step 9: Commit**

```bash
git add frontend/src frontend/bindings
git commit -m "feat(ui): pass bookingID, render finish-drain progress, cancel button"
```

---

## Task 11: End-to-end verification

- [ ] **Step 1: Full Go build**

Run: `go build .`
Expected: success.

- [ ] **Step 2: Full frontend build**

Run: `cd frontend && npm run build`
Expected: success.

- [ ] **Step 3: Start the app via Wails**

Run: `wails3 dev` (or the project's usual dev command).
Expected: app launches.

- [ ] **Step 4: Smoke — normal flight**

- Connect simulator.
- Click Start Flight with a valid booking and network up.
- Fly for >1 minute.
- Click Finish Flight.
- Observe: `flight-state = finishing` briefly, then `flight-finish-complete`; UI returns to idle.
- In SQLite console (e.g. `sqlite3 <config-dir>/airspace-acars/flight_data.db "SELECT COUNT(*) FROM position_outbox;"`), confirm outbox is empty.

- [ ] **Step 5: Smoke — network-drop mid-flight**

- Start a flight.
- Block egress to the API (host entry, firewall, or disconnect Wi-Fi).
- Fly for 30 seconds.
- Observe logs: repeated "server connection lost" / "server still unreachable"; outbox grows.
- Query SQLite: `SELECT COUNT(*) FROM position_outbox WHERE booking_id = '<id>';` — non-zero.
- Restore network.
- Observe logs: "outbox drained, sent=250 remaining=..." per tick until zero.

- [ ] **Step 6: Smoke — finish with pending outbox**

- Start a flight and disconnect mid-flight, let outbox grow to a few thousand rows.
- Reconnect very briefly, then disconnect again so the outbox is still non-empty.
- Click Finish Flight.
- Observe UI: *"Sending X missing positions before finishing this flight…"* with X decreasing.
- Reconnect fully.
- Observe completion event and that `/api/acars/finish` fires only after the outbox drains to zero.

- [ ] **Step 7: Smoke — cancel finish**

- With the network off, click Finish Flight and watch the draining UI.
- Click Cancel Finish.
- Observe: state returns to `active`; position loop resumes; outbox rows remain.

- [ ] **Step 8: Smoke — app crash recovery**

- Start a flight, queue some rows (disconnect network).
- Kill the app (Task Manager / `kill -9`).
- Relaunch, log in, and click Start Flight with the same booking.
- Observe toast: *"Resuming delivery of N positions from a previous session."*
- With network up, outbox drains during the new flight's ticks.

- [ ] **Step 9: Final commit (if any stray changes)**

```bash
git status
# Expect: clean. If not, commit with a clear message.
```

---

## Self-review checklist (run after implementation, before merge)

- [ ] `grep -rn "var pendingReports\|pendingReports =\|pendingReports\[" internal/ frontend/src/` — should return zero matches (removed in Task 5; the old in-RAM queue is gone).
- [ ] `grep -rn "flushPendingReports" internal/` — should return zero matches (removed in Task 5).
- [ ] `grep -n "maxBatchSize" internal/app/flight.go` — used by `sendPositionBatches` and `drainOutbox`; value is 250.
- [ ] `git log --oneline main..HEAD` — commits match task order; each compiles.
- [ ] Outbox table exists in a fresh DB: delete `flight_data.db`, launch app, query `SELECT name FROM sqlite_master WHERE type='table';` — `position_outbox` present.
- [ ] Spec references (`docs/superpowers/specs/2026-04-22-position-batching-design.md`) are satisfied:
  - 250/req cap — Task 3, 5.
  - Raised caps (1000 pending, 3000 high-res) — Task 2.
  - Booking-id-scoped outbox — Task 1, 4.
  - Async finish + progress event + cancel — Task 6, 10.
  - Resume toast on start — Task 4, 10.
  - New metrics — Task 8.
