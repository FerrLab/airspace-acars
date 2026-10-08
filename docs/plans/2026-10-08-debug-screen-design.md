# Debug screen: runway, stand and the report actually sent

## Goal

The debug tab should answer the two questions a runway/stand report raises,
without starting a flight or reading logs:

1. **Where does the ACARS think the aircraft is?** Airport, runway, stand, and
   whether the last airport lookup worked.
2. **What did it send?** The exact position report the uploader handed to the
   API, when, and what became of it: sent, queued to the outbox, or dropped.

The tab is also reorganised. Today it is one long scroll of 13 tables in two
arbitrary columns, with the logs and the payload at the bottom. Its row labels
are hard-coded English, and its "API payload" is rebuilt in the webview: it
lacks `acarsVersion`, `simulator`, `engines[].exists`, `runway` and `stand`,
shows a placeholder callsign, and writes the timestamp as an ISO string where
the real report sends epoch milliseconds.

Prerequisite: the MSFS Facilities decoding fix (branch `fix/msfs-runway-stand`)
lands first, as its own PR. Without it every MSFS lookup returns "no airport"
and this screen would only show that.

## Decisions taken

| Question | Decision |
|---|---|
| When is runway/stand shown? | Whenever a simulator is connected, flight or not |
| Which report is shown? | The last one the uploader handled, with its outcome; a marked preview before a flight |
| How does the screen get it? | It polls one typed service call while the tab is mounted |
| Layout | Status strip, then tabs: Overview / Telemetry / Payload / Logs |

## Backend

### Ground locator at connection lifetime

`groundLocator` (`internal/app/ground_locator.go`) today lives inside the
flight position loop (`flight.go`), so nothing looks an airport up until a
flight is started.

- It is started and stopped with the data stream (`startDataStreamLocked` /
  `stopDataStreamLocked` in `sim.go`). The App field holding it is guarded by
  the existing `simMu`; the locator keeps its own `mu`. No new mutex on `App`.
- `dataStreamLoop` calls `Observe` once per 1 Hz tick. `Observe` already never
  blocks and already rate-limits lookups (`layoutRetryEvery`, 30 s), so the
  sim sees no more lookups than a flight already causes.
- The flight loop takes the running locator instead of starting its own.
  `Observe` is safe from both goroutines: state is under `mu`, and `fetching`
  keeps lookups to one at a time.
- `Observe` stores the fix it computed. A new `Status()` returns
  `domain.GroundStatus`:

  ```go
  type GroundStatus struct {
      Airport   string    `json:"airport"`   // ICAO, "" when none
      Runways   int       `json:"runways"`
      Stands    int       `json:"stands"`
      Runway    string    `json:"runway"`    // "36/18", "" when not on one
      Stand     string    `json:"stand"`     // "7", "" when not on one
      LoadedAt  time.Time `json:"loadedAt"`  // zero when no layout
      CheckedAt time.Time `json:"checkedAt"` // when the last lookup finished, zero before any
      LastError string    `json:"lastError"` // last lookup failure, "" after a success
  }
  ```

  `CheckedAt` tells "not looked up yet" from "looked up, no airport here":
  the Overview says "no airport nearby" only once a lookup has said so.
  (Added after the final review; without it the two read the same, which is
  how the broken MSFS decoding would have looked.)

  `LastError` carries the adapter's error text, which names an ICAO and a
  timeout at most; nothing pilot-identifying (AGENTS §4.3).

### Report tap

A new type `reportTap` in `internal/app/report_tap.go`, with its own lock,
held by `App` and handed to `positionUploader`.

- `positionUploader.send` records the batch's last report with outcome
  `sent` on success, `outbox` when it falls back to `persist`. `persist`
  records `dropped` when there is no booking to file under. Also stored: the
  time and the batch size.
- It keeps a reference to the report map itself. `buildPositionReport` makes
  a fresh map per report and nothing mutates it after it is submitted, so
  holding it is safe. It is encoded with `json.MarshalIndent` only when
  `Snapshot()` is called: the same `measurement` encoder the upload uses, and
  no cost while nobody is looking.
- Outside a flight, `dataStreamLoop` stores a preview built by the same
  `buildPositionReport(fd, locator.Observe(fd))`, outcome `preview`. During a
  flight it does not, so a real report is never overwritten by a preview.

```go
type PositionReportSnapshot struct {
    JSON      string    `json:"json"`      // indented report, "" before any
    At        time.Time `json:"at"`
    Outcome   string    `json:"outcome"`   // sent | outbox | dropped | preview
    BatchSize int       `json:"batchSize"` // reports in the POST it came from
}
```

### Service

`DebugService` in `services.go`:

```go
func (s *DebugService) GetDebugSnapshot() domain.DebugSnapshot

type DebugSnapshot struct {
    Ground GroundStatus           `json:"ground"`
    Report PositionReportSnapshot `json:"report"`
}
```

Registered in `main.go`. A typed struct, not a map (AGENTS §3.4). Bindings
regenerated; `frontend/src/__mocks__/wails-bindings.ts` gains its mock.

### Observability

The snapshot call is a cheap read and gets no span. The locator's existing
`airport.layout.fetch` span and `airport.layout_*` counters cover lookups,
which now happen outside flights too.

## Webview

`debug-tab.tsx` (484 lines) becomes a container under 100 lines.

```
components/debug/
  debug-status-strip.tsx   connection · adapter · aircraft · profile ·
                           airport/stand · last report age and outcome
  debug-overview.tsx       airport, runway, stand, lookup state,
                           position/speeds/lights summary
  debug-telemetry.tsx      the existing tables, grouped:
                           flight · engines & systems · radios & autopilot ·
                           lights & controls
  debug-payload.tsx        snapshot JSON, outcome, time, batch size, Copy
  debug-logs.tsx           the existing LogViewer, moved
hooks/use-debug-snapshot.ts  polls GetDebugSnapshot every 1 s while mounted;
                             cancelled flag in cleanup (AGENTS §5)
lib/debug-format.ts          pure formatting (ages, outcomes, runway labels)
```

- Tabs from `components/ui/tabs.tsx` (already installed).
- The webview-built payload is deleted.
- Every visible string goes through `t("debug.*")` in en, es, pt and fr,
  including the table row labels. `locale-parity.test.ts` gains a `debug.`
  check.
- Event payloads and the snapshot are typed; no new `any`.

## Testing

Go:
- `Status()` reports the airport, runway and stand from the last `Observe`;
  reports the lookup error, then clears it after a success.
- The data stream drives the locator: a fake provider sees a lookup while
  connected and no flight is active.
- `reportTap`: a successful send records `sent` with the exact report; a
  failed send records `outbox`; no booking records `dropped`; a preview never
  replaces a real report during a flight; `Snapshot()` encodes NaN as null.

Webview (React Testing Library, mocked bindings):
- The overview shows SBRF, stand "7" and no runway from a mocked snapshot.
- The payload tab shows the JSON, its outcome and age; Copy writes it.
- The strip shows connected/disconnected and the last report's outcome.
- `lib/debug-format.ts` unit tests.

## Docs

- `docs/runway-stand.md`: lookups run whenever a simulator is connected, and
  the debug tab shows their result.
- README feature list: the debug tab shows ground position and the last sent
  report.

## Out of scope

- A history of reports.
- Changing the lookup cadence or radius.
- X-Plane-specific work: X-Plane uses the same locator and gets the screen
  as is.
