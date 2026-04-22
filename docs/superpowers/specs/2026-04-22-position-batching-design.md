# Position Batching Design

**Date:** 2026-04-22
**Area:** `internal/app/flight.go`
**Status:** Approved (pending implementation)

## Problem

When position reports accumulate (during flare high-res collection, or during a
server outage that fills the pending queue), the current code sends the entire
accumulated slice in a single HTTP POST. A single request can carry up to 1500
high-res reports, which risks overflowing the backend's request size or
processing budget.

## Goal

Cap any single `POST /api/v2/acars/position` request to at most **1000 reports**
per request, applied at every site that can ship more than one report at a time.

## Non-goals

- No change to in-memory queue caps (`maxPendingReports = 500`,
  `maxHighResReports = 1500`).
- No change to the normal per-tick single-report POST — it is already ≤1 report.
- No change to the backend contract — it already accepts either a single object
  or an array.
- No new retry logic inside the batching helper — callers already handle
  retry/queue-and-defer semantics.

## Design

### Constant

```go
const maxBatchSize = 1000
```

Added alongside the other flight constants (near `maxHighResReports`).

### Helper

New method on `*App`, placed near `doRequestWithRetry`:

```go
// sendPositionBatches POSTs reports to /api/v2/acars/position in chunks of at
// most maxBatchSize. Returns how many reports were sent successfully before
// either finishing or hitting an error. Stops on the first error so callers
// can keep the unsent suffix queued.
func (a *App) sendPositionBatches(reports []map[string]interface{}) (int, error)
```

Semantics:
- Iterates `reports` in slices of up to `maxBatchSize`.
- On success of a batch, increments `sent` by the batch length.
- On error, returns `(sent, err)` immediately — caller decides what to do with
  the unsent suffix.
- Empty input returns `(0, nil)`.

### Call-site changes

Four sites convert from their current pattern to the helper:

| Site (approx. line)                              | Today                                                  | After                                                                                     |
| ------------------------------------------------ | ------------------------------------------------------ | ----------------------------------------------------------------------------------------- |
| High-res flush on flight end (line 500)          | Single POST of full `highResQueue`                     | `sent, err := a.sendPositionBatches(highResQueue)`; log sent/remaining; increment metrics |
| High-res drain during ticker (line 628)          | Single POST of full `highResQueue`                     | Batch; `highResQueue = highResQueue[sent:]`; if empty assign `nil`; metrics per success   |
| `pendingReports` resend during ticker (line 611) | One-by-one loop, break on first error                  | Single call to helper; `pendingReports = pendingReports[sent:]`; nil-on-empty             |
| `flushPendingReports` on flight end (line 662)   | One-by-one loop, early-return on first error           | Single call to helper; on error log remaining; on success log count                       |

### Slice-pinning

After truncating a queue with `queue = queue[sent:]`, the old backing array
stays alive. When `len(queue) == 0` the callers also assign `nil` to release
it. This is a small memory hygiene fix that matters in a long-lived loop.

### Metrics

- `posReportsSent` — incremented by actual `sent` (whether partial or full),
  exactly as today.
- `posReportsFailed` — on error, incremented by `len(input) - sent` so we now
  record the precise failure count instead of the whole-queue count.
- `posReportsQueued` — unchanged; only the tick-time single-report failure path
  queues new reports.
- `posFlushTotal` — success attribute is `err == nil` as today.
- `posHighResDepth` — recorded before the drain, unchanged.

## Error handling

All errors are handled at the call site, not the helper. The helper does not
log — callers already have their own context-specific log lines
(`"server connection lost"`, `"failed to flush queued report on flight end"`,
etc.). The helper is a pure mechanical splitter.

## Testing

No automated tests exist for `flight.go`. Validation will be manual:
- Build: `go build .`
- Smoke: run a short flight, trigger flare, confirm batching via logs.
- (Optional) temporarily lower `maxBatchSize` and verify multiple POSTs go out
  when the high-res queue has >1 batch worth of reports.

## Files changed

- `internal/app/flight.go` — constant, helper, four call-site edits.

## Out of scope

- No new tests.
- No frontend change.
- No change to observability schema beyond the existing counters.
