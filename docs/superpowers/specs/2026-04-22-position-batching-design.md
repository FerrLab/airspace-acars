# Durable Position Outbox + Batched Delivery

**Date:** 2026-04-22
**Area:** `internal/app/flight.go`, `internal/adapters/storage/sqlite.go`,
`internal/app/app.go`, `frontend/src/components/acars-tab.tsx`
**Status:** Approved (pending implementation)

## Problem

Today position reports live only in RAM. Two problems compound:

1. A single `POST /api/v2/acars/position` can ship up to `maxHighResReports`
   (1500) reports at once, risking backend overflow.
2. If the server is unreachable when a user clicks "Finish flight", up to 500
   queued reports are flushed best-effort and silently lost on error.

## Goals

1. Cap every `POST /api/v2/acars/position` at **250 reports** per request.
2. Persist every report that cannot be sent immediately in a SQLite **outbox**,
   keyed by `booking_id`, so nothing is lost across crashes or disconnects.
3. **Never finish a flight with pending positions** — `FinishFlight` blocks
   (asynchronously, via events) until the outbox for that booking is empty,
   retrying forever with backoff on transient errors. The user can cancel.
4. Show the user a live progress message:
   *"Sending X missing positions before finishing this flight…"*
5. Raise capacities: `maxPendingReports = 1000`, `maxHighResReports = 3000`.

## Non-goals

- No change to the per-tick normal single-report POST — still ≤1 report.
- No change to the backend contract — endpoint already accepts an array.
- No outbox compression, encryption, or vacuuming beyond `DELETE`.
- No automatic drain on app startup (only on start/finish flight).
- No cross-booking drain — each booking's rows drain independently.

## Design

### 1. Constants

```go
const (
    maxBatchSize         = 250    // per-request cap
    maxPendingReports    = 1000   // was 500
    maxHighResReports    = 3000   // was 1500
    finishDrainTickEvery = 1 * time.Second  // UI progress event cadence
)
```

### 2. Storage layer — outbox table and methods

**Schema migration** in `internal/adapters/storage/sqlite.go`:

```sql
CREATE TABLE IF NOT EXISTS position_outbox (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    booking_id TEXT    NOT NULL,
    payload    TEXT    NOT NULL,   -- JSON of the position report
    created_at INTEGER NOT NULL    -- unix millis
);
CREATE INDEX IF NOT EXISTS idx_outbox_booking ON position_outbox(booking_id, id);
```

**`Storage` port** extended in `internal/app/app.go`:

```go
type Storage interface {
    SaveFlightData(*domain.FlightData) error
    QueryFlightData() (*sql.Rows, error)
    PurgeFlightData() error

    // Position outbox
    EnqueuePosition(bookingID string, payload []byte) error
    PeekOutboxBatch(bookingID string, limit int) (ids []int64, payloads [][]byte, err error)
    DeleteOutboxBatch(ids []int64) error
    CountOutbox(bookingID string) (int, error)
}
```

SQLite implementations serialize writers with a `sync.Mutex` to avoid
contention against `SaveFlightData`. `PeekOutboxBatch` orders by `id ASC` so
reports ship in insertion order.

### 3. App layer — booking ID plumbing and state

**`StartFlight` signature** gains `bookingID`:

```go
func (a *App) StartFlight(callsign, departure, arrival, bookingID string) error
```

Callers:
- `internal/app/sim.go:tryAutoStartFlight` — booking is fetched there; extract
  `booking["id"]` (string or number → string).
- Frontend `handleStartFlight` — already holds `booking`; pass `booking.id`.

**State values**:

```go
state string // "idle" | "active" | "finishing"
```

`bookingID` is stored in the `App` struct alongside `callsign`/`departure`/etc.

### 4. Positionloop — outbox-based write path

Tick-by-tick behavior:

| Step                           | Today                                    | New                                                                      |
| ------------------------------ | ---------------------------------------- | ------------------------------------------------------------------------ |
| Single-report POST             | On fail, append to in-RAM pendingReports | On fail, `EnqueuePosition(bookingID, payload)`                           |
| In-RAM `pendingReports` drain  | Loop one-by-one                          | **Removed** — drain path now goes through outbox                         |
| `highResQueue` drain           | Single POST of full slice                | Send in batches of 250 via `sendPositionBatches`; failed → `EnqueuePosition` per row |
| Outbox drain per tick          | —                                        | `a.drainOutbox(bookingID, 1)` — one batch of 250 per tick                |
| Stop loop (endFlight)          | Flush in-RAM                             | Persist remaining in-RAM `highResQueue` to outbox before returning       |

The `highResQueue` stays in-memory as today (cap 3000) for instantaneous flare
capture; it's only persisted to the outbox when (a) a batch POST fails, or
(b) the loop is stopping.

### 5. `sendPositionBatches` helper

```go
// sendPositionBatches POSTs reports to /api/v2/acars/position in chunks of at
// most maxBatchSize. Returns the number sent successfully before returning.
// On error, returns (sent, err) and leaves the unsent suffix to the caller.
func (a *App) sendPositionBatches(reports []map[string]interface{}) (int, error)
```

Callers decide what to do with unsent suffix (re-enqueue, shift slice, etc.).

### 6. `drainOutbox` helper

```go
// drainOutbox sends up to `maxBatches` batches of maxBatchSize from the outbox
// for the given booking. Returns (sent, remaining, err). Stops on the first
// batch failure, leaving unsent rows in the DB.
func (a *App) drainOutbox(bookingID string, maxBatches int) (sent, remaining int, err error)
```

On each successful batch: `DeleteOutboxBatch(ids)` to remove the shipped rows.
Rows only leave the DB after the server has 200'd them.

### 7. `StartFlight` — non-blocking drain of leftover outbox

After the successful `/api/acars/start` call and state transition to `active`:

1. Check `CountOutbox(bookingID)`. If >0, log and emit a toast event
   `flight-outbox-resuming` with `{pending: count}`.
2. Do **not** block start. The position loop will drain in the normal
   one-batch-per-tick cadence.

Rationale: blocking start on drain would make the app unusable when the API
is down. Drain-during-flight is safe because each tick drains one batch.

### 8. `FinishFlight` — asynchronous, durable

**Synchronous phase** (under `flightMu`):
1. Validate preconditions (state == "active", min flight duration) — as today.
2. Set `state = "finishing"`, emit `flight-state = "finishing"`.
3. Stop the position loop (`close(a.stopCh)`) — which persists remaining
   in-RAM `highResQueue` into the outbox.
4. Spawn `a.finishDrainLoop(bookingID, callsign, departure, arrival)`.
5. Return `nil` immediately. The UI is event-driven from here.

**`finishDrainLoop` goroutine**:

```
loop:
    count, _ := a.DB.CountOutbox(bookingID)
    if count == 0:
        body, status, err := a.doRequestWithRetry("POST", "/api/acars/finish", payload)
        if err != nil || status >= 400:
            emit flight-finish-failed { reason: ... }
            state = "active"   // revert so user can try again
            return
        emit flight-finish-complete
        state = "idle"; clear booking fields
        return

    emit flight-finish-progress { pending: count }

    sent, _, err := a.drainOutbox(bookingID, 4)   // 4 batches = 1000 rows per pass
    if err != nil:
        sleep backoff (2s doubling up to 60s, capped)
    else:
        sleep finishDrainTickEvery
```

The loop also listens on an `a.finishCancelCh` — if closed, it exits without
calling `/finish`, leaves outbox intact, and reverts state to `active`.

### 9. `CancelFinish` (new command)

```go
func (a *App) CancelFinish() error
```

- Valid only when `state == "finishing"`.
- Closes `a.finishCancelCh`, reverts state to `active`, restarts the position
  loop. Outbox rows remain. User can retry `FinishFlight` later.

### 10. UI changes (`acars-tab.tsx`)

- Pass `booking.id` to `FlightService.StartFlight`.
- Subscribe to new events:
  - `flight-state = "finishing"` → disable buttons, show drain panel.
  - `flight-finish-progress { pending }` → render *"Sending X missing
    positions before finishing this flight…"* (via `t("acars.finishingDrain",
    { count: pending })`).
  - `flight-finish-complete` → success toast, return to idle.
  - `flight-finish-failed { reason }` → alert; state reverts to active.
  - `flight-outbox-resuming { pending }` → toast on start:
    *"Resuming delivery of X positions from a previous session."*
- Add a "Cancel finish" button while in `finishing` state → calls
  `FlightService.CancelFinish()`.
- i18n keys added to `frontend/src/i18n/` (or equivalent).

### 11. Metrics

Added:
- `position.outbox_depth` (histogram) — recorded per tick and per drain pass.
- `position.outbox_enqueued` (counter) — increments on `EnqueuePosition`.
- `flight.finish_drain_duration_sec` (histogram) — time from finish click to
  `/finish` success.
- `flight.finish_canceled_total` (counter) — `CancelFinish` calls.

Retained:
- `posReportsSent`, `posReportsFailed`, `posReportsQueued` (now reflects
  outbox enqueues), `posFlushTotal`, `posHighResQueued`, `posHighResDepth`.

Removed:
- `posQueueDepth` (in-RAM pending queue no longer exists) — or repurposed to
  track outbox depth; see implementation plan.

### 12. Failure behavior summary

| Failure                              | Behavior                                                                   |
| ------------------------------------ | -------------------------------------------------------------------------- |
| Single-report POST fails mid-flight  | Enqueue to outbox; metric `posReportsQueued`; logs throttled               |
| High-res batch POST fails            | Row-by-row enqueue to outbox; counters bumped                              |
| Outbox drain batch POST fails        | Rows stay in DB; next tick retries                                         |
| App crashes mid-flight               | Outbox rows survive; drained on next `StartFlight` with same `bookingID`   |
| Server down at finish click          | `finishDrainLoop` retries forever; user sees progress; may `CancelFinish`  |
| `/api/acars/finish` 4xx after drain  | `flight-finish-failed` event; state reverts to `active`                    |
| DB write fails                       | Logged, counter incremented; reports lost for that tick (SQLite down ≈ disk full; no better option) |

## Testing

No automated tests exist for `flight.go` today. Validation plan:

1. `go build .` — compile clean.
2. Manual smoke:
   - Start flight with network up → confirm `flight-state = active`, no outbox growth.
   - Disconnect network mid-flight → observe outbox grow via logs.
   - Reconnect → observe drain (250/tick) via logs.
   - Click Finish with outbox empty → normal finish.
   - Click Finish with outbox non-empty → progress UI, then completion.
   - Kill app mid-flight → restart, start same booking → observe resume toast + drain.
   - Click Finish with network down → progress stays, test Cancel → returns to active.
3. Optional: temporarily lower `maxBatchSize` to 5 to exercise batch loop without
   a 3000-report flare.

## Files changed

- `internal/adapters/storage/sqlite.go` — migration + 4 new methods.
- `internal/app/app.go` — extend `Storage` interface; add `finishCancelCh`,
  `bookingID` fields.
- `internal/app/flight.go` — constants, helpers, rewritten positionLoop
  write/drain paths, async finish, `CancelFinish`.
- `internal/app/sim.go` — pass booking id into `StartFlight`.
- `internal/ports/user_action.go` — expose `CancelFinish`; update `StartFlight` signature.
- `services.go` — regenerate/update Wails service signatures.
- `frontend/src/components/acars-tab.tsx` — booking id, new events, progress UI,
  cancel button.
- `frontend/src/__mocks__/wails-bindings.ts` — mock new methods/events.
- i18n files — new keys.

## Open items handled at implementation time

- Booking id coercion: server may return number or string — normalize to string
  at the boundary.
- Outbox row retention after a successful finish: rows are deleted per batch
  as they ship, so the table for that booking is empty on successful finish.
  No cleanup job needed.
- Frontend bindings regen: `wails3 generate bindings` after Go signature changes.
