# Debug Screen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The debug tab shows where the ACARS places the aircraft (airport, runway, stand) whenever a simulator is connected, and the exact position report the uploader last handled with its outcome, in a reorganised tabbed layout.

**Architecture:** The ground locator moves from flight lifetime to the data stream's lifetime and remembers its last fix. A new `reportTap` records what the uploader did with each batch, plus a preview outside flights. `DebugService.GetDebugSnapshot()` returns both as one typed struct, polled once a second by the tab while it is mounted. The 484-line `debug-tab.tsx` becomes a container over five small components.

**Tech Stack:** Go 1.x (`internal/app`, `internal/domain`), Wails v3 services and bindings, React + TypeScript, radix-ui Tabs (shadcn), i18next, Vitest + React Testing Library + user-event.

**Spec:** `docs/plans/2026-10-08-debug-screen-design.md`

## Global Constraints

- Dependency direction domain ← app ← adapters ← main; no new mutex on `App` (AGENTS §2, §3.2).
- Service returns are typed `domain` structs with JSON tags, never maps (AGENTS §3.4).
- Never call out (HTTP, SQLite, `UI.EmitEvent`, another lock, `Stop()`) while holding a mutex (AGENTS §4.4).
- Every goroutine that outlives its caller begins with `defer observability.Recover()`.
- Every user-visible string goes through `t("debug.*")` and exists in `en`, `es`, `pt` and `fr`; `locale-parity.test.ts` checks the `debug.` prefix (AGENTS §5).
- Async effects set a `cancelled` flag in cleanup and check it before every `setState`; event unsubscribe returned from the effect (AGENTS §5).
- New frontend code introduces no `any`.
- Go files you touch: `gofmt` clean (normalise CRLF before checking on Windows), `go vet` clean.
- Commits: Conventional Commits, scope `frontend` or the feature area; body says why; trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Windows note: Kaspersky blocks `go test`'s default `simconnect.test.exe`; for that package build with `go test -c -o <scratch>/x.exe` and run the binary.

## Review Focus

1. **Simulator reconnect mid-flight.** The flight loop must pick up the new connection's locator, not keep a stopped one whose `fetching` flag never clears. Task 1 Step 1 pins it (`TestAReconnectGetsAFreshLocator`), and the flight loop calls `a.currentLocator()` per sample.
2. **Go's zero `time.Time` (`"0001-01-01T00:00:00Z"`).** The screen must say "not loaded" / "no report yet", not "2025 years ago". Pinned in Task 3 (`isUnsetTime`, `secondsSince`).
3. **A NaN reading in a previewed or kept report.** The snapshot shows `null`, not an encode error. Pinned in Task 2 (`TestASnapshotEncodesABadReadingAsNull`).
4. **Tab unmounted.** Polling stops, so a closed debug tab costs nothing. Pinned in Task 3 (`stops polling when unmounted`).
5. **Clipboard unavailable in the webview.** Copy does nothing rather than throw. Pinned in Task 4 (`copy does nothing without a clipboard`).

---

### Task 1: Ground status and connection-lifetime locator (Go)

**Files:**
- Create: `internal/domain/debug.go`
- Modify: `internal/app/ground_locator.go` (struct fields, `Observe`, `fetch`, new `Status`, new `currentLocator`)
- Modify: `internal/app/app.go:69-78` (App sim fields)
- Modify: `internal/app/sim.go:315-367` (`dataStreamLoop`)
- Modify: `internal/app/flight.go:690-697, 717, 815` (position loop)
- Test: `internal/app/ground_locator_test.go`

**Interfaces:**
- Produces: `domain.GroundStatus`, `domain.PositionReportSnapshot`, `domain.DebugSnapshot`, constants `domain.ReportSent|ReportOutbox|ReportDropped|ReportPreview`; `func (g *groundLocator) Status() domain.GroundStatus` (nil-safe); `func (a *App) currentLocator() *groundLocator`; App field `locator *groundLocator` (guarded by `simMu`).

- [ ] **Step 1: Write the failing tests** — append to `internal/app/ground_locator_test.go` (add `"airspace-acars/internal/profiles"` to its imports):

```go
// The debug screen reads where the locator last placed the aircraft.
func TestStatusNamesTheAirportAndWhereTheAircraftIs(t *testing.T) {
	l := newTestLocator(&fakeLayouts{layout: egll()})
	if s := l.Status(); s.Airport != "" || !s.LoadedAt.IsZero() {
		t.Fatalf("status before any lookup = %+v, want empty", s)
	}

	l.Observe(onRunway09L())
	l.serve()
	l.Observe(onRunway09L())

	s := l.Status()
	if s.Airport != "EGLL" || s.Runways != 1 || s.Runway != "09L/27R" || s.Stand != "" {
		t.Fatalf("status = %+v, want EGLL with 1 runway, on 09L/27R, no stand", s)
	}
	if !s.LoadedAt.Equal(l.clock) {
		t.Errorf("layout loaded at %v, want %v", s.LoadedAt, l.clock)
	}

	climbing := onRunway09L()
	climbing.Sensors.OnGround = false
	climbing.Position.AltitudeAGL = 500
	l.Observe(climbing)
	if s := l.Status(); s.Runway != "" || s.Airport != "EGLL" {
		t.Fatalf("after lift-off, status = %+v; want EGLL kept and no runway", s)
	}

	var none *groundLocator
	if s := none.Status(); s.Airport != "" {
		t.Fatalf("a nil locator reported %+v", s)
	}
}

// A failed lookup stays on screen until one works. "No airport here" is an
// answer, not a failure, and shows no error.
func TestStatusShowsTheLastLookupFailureUntilOneWorks(t *testing.T) {
	captureLogs(t)
	sim := &fakeLayouts{err: errors.New("facility data for SBRF incomplete after 8s")}
	l := newTestLocator(sim)

	l.Observe(onRunway09L())
	l.serve()
	if s := l.Status(); s.LastError != "facility data for SBRF incomplete after 8s" {
		t.Fatalf("last error = %q, want the lookup's failure", s.LastError)
	}

	sim.err, sim.layout = nil, egll()
	l.clock = l.clock.Add(layoutRetryEvery)
	l.Observe(onRunway09L())
	l.serve()
	if s := l.Status(); s.LastError != "" || s.Airport != "EGLL" {
		t.Fatalf("after a successful lookup, status = %+v", s)
	}

	sim.err, sim.layout = domain.ErrNoAirportData, nil
	far := onRunway09L()
	far.Position.Latitude = 40
	l.clock = l.clock.Add(layoutRetryEvery)
	l.Observe(far)
	l.serve()
	if s := l.Status(); s.LastError != "" || s.Airport != "" || !s.LoadedAt.IsZero() {
		t.Fatalf("with no airport around, status = %+v, want empty with no error", s)
	}
}

// The locator runs for as long as a simulator is connected, flight or not,
// so the stand can be checked at the gate before departure.
func TestTheLocatorRunsWhileASimulatorIsConnected(t *testing.T) {
	sim := &fakeSim{answering: true}
	a, _ := newSimApp(t, sim)
	a.profileRegistry = profiles.NewRegistry()

	if a.currentLocator() != nil {
		t.Fatal("a locator was running before any simulator connected")
	}
	a.autoConnect()
	waitFor(t, "the locator starts with the connection", func() bool { return a.currentLocator() != nil })

	a.DisconnectSim()
	waitFor(t, "the locator stops with the connection", func() bool { return a.currentLocator() == nil })
}

// A reconnect replaces the locator. The flight loop asks for the current one
// on every sample; a stopped one would never look an airport up again.
func TestAReconnectGetsAFreshLocator(t *testing.T) {
	stalled := &fakeSim{answering: true}
	fresh := &fakeSim{answering: true}
	a, _ := newSimApp(t, stalled, fresh)
	a.profileRegistry = profiles.NewRegistry()

	a.autoConnect()
	waitFor(t, "the first connection's locator", func() bool { return a.currentLocator() != nil })
	first := a.currentLocator()

	stalled.set(func(f *fakeSim) { f.stale = true })
	waitFor(t, "the stale connection is dropped", func() bool { return a.currentLocator() == nil })
	a.autoConnect()
	waitFor(t, "the new connection's locator", func() bool {
		l := a.currentLocator()
		return l != nil && l != first
	})
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/ -run 'Status|Locator' -count=1`
Expected: build failure, `l.Status undefined` and `a.currentLocator undefined`.

- [ ] **Step 3: Add the domain types** — create `internal/domain/debug.go`:

```go
package domain

import "time"

// GroundStatus is where the ACARS places the aircraft on an airport, for the
// debug screen: the airport whose layout it holds and the runway or stand the
// aircraft was last found on.
type GroundStatus struct {
	Airport   string    `json:"airport"`   // ICAO, "" when no layout is held
	Runways   int       `json:"runways"`   // in the held layout
	Stands    int       `json:"stands"`    // in the held layout
	Runway    string    `json:"runway"`    // "36/18", "" when not on one
	Stand     string    `json:"stand"`     // "7", "" when not on one
	LoadedAt  time.Time `json:"loadedAt"`  // zero when no layout is held
	LastError string    `json:"lastError"` // the last lookup's failure, "" after a success
}

// What became of a position report the debug screen shows.
const (
	ReportSent    = "sent"    // the API accepted it
	ReportOutbox  = "outbox"  // it failed and waits in the outbox
	ReportDropped = "dropped" // it had no booking to be filed under
	ReportPreview = "preview" // no flight: the report that would be sent
)

// PositionReportSnapshot is a position report as the uploader handled it:
// the report itself, encoded the way it was sent, and its fate.
type PositionReportSnapshot struct {
	JSON      string    `json:"json"`      // indented report, "" before any
	At        time.Time `json:"at"`        // when it was handled
	Outcome   string    `json:"outcome"`   // one of the Report* values
	BatchSize int       `json:"batchSize"` // reports in the request it came from
}

// DebugSnapshot is everything the debug screen polls for.
type DebugSnapshot struct {
	Ground   GroundStatus           `json:"ground"`
	Report   PositionReportSnapshot `json:"report"`
	Profiles []string               `json:"profiles"` // active aircraft profile IDs
}
```

- [ ] **Step 4: Make the locator remember its fix and report status** — in `internal/app/ground_locator.go`:

Change the struct's lock comment and fields:

```go
	// mu guards layout, lastAttempt, fetching, failures, fix, loadedAt and
	// lastErr.
	mu          sync.Mutex
	layout      *domain.AirportLayout
	lastAttempt time.Time
	fetching    bool
	failures    int
	fix         groundFix // the last Observe's answer, for Status
	loadedAt    time.Time
	lastErr     string
```

Rename the existing `Observe` body to an unexported `observe` (same body, minus its `g == nil || fd == nil` guard) and put this `Observe` in its place:

```go
// Observe places the aircraft on the airport. It never blocks: when the
// layout it needs is not in hand it asks for it and reports nothing this
// time.
func (g *groundLocator) Observe(fd *domain.FlightData) groundFix {
	if g == nil || fd == nil {
		return groundFix{}
	}
	fix := g.observe(fd)
	g.mu.Lock()
	g.fix = fix
	g.mu.Unlock()
	return fix
}
```

In `fetch`, inside the existing locked block right after `g.layout = layout`, add:

```go
	g.loadedAt, g.lastErr = time.Time{}, ""
	switch {
	case err == nil:
		g.loadedAt = g.now()
	case !errors.Is(err, domain.ErrNoAirportData):
		g.lastErr = err.Error()
	}
```

Add after `Stop`:

```go
// Status is what the debug screen shows about the locator: the airport it
// holds and where it last placed the aircraft.
func (g *groundLocator) Status() domain.GroundStatus {
	if g == nil {
		return domain.GroundStatus{}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	s := domain.GroundStatus{LoadedAt: g.loadedAt, LastError: g.lastErr}
	if l := g.layout; l != nil {
		s.Airport, s.Runways, s.Stands = l.ICAO, len(l.Runways), len(l.Stands)
	}
	if r := g.fix.Runway; r != nil {
		s.Runway = r.Ends[0].Designator + "/" + r.Ends[1].Designator
	}
	if st := g.fix.Stand; st != nil {
		s.Stand = st.Name
	}
	return s
}
```

Add after `layoutProvider`:

```go
// currentLocator returns the running data stream's locator, or nil while no
// simulator is connected. Callers ask per use: a reconnect replaces it.
func (a *App) currentLocator() *groundLocator {
	a.simMu.Lock()
	defer a.simMu.Unlock()
	return a.locator
}
```

- [ ] **Step 5: Add the App field** — in `internal/app/app.go`, in the "Sim connection state" block after `streamStopCh`:

```go
	// locator is the running data stream's ground locator, nil while no
	// simulator is connected. Guarded by simMu; the locator locks itself.
	locator *groundLocator
```

- [ ] **Step 6: Run the locator with the data stream** — in `internal/app/sim.go`, at the top of `dataStreamLoop` right after `defer observability.Recover()`:

```go
	// The ground locator lives as long as this stream, so runway and stand
	// are known at the gate, before any flight. It is published for the
	// flight loop and the debug screen, and withdrawn only if no newer
	// stream has replaced it.
	locator := a.startGroundLocator()
	a.simMu.Lock()
	a.locator = locator
	a.simMu.Unlock()
	defer func() {
		a.simMu.Lock()
		if a.locator == locator {
			a.locator = nil
		}
		a.simMu.Unlock()
		locator.Stop()
	}()
```

and right after `a.UI.EmitEvent("flight-data", data)`:

```go
			locator.Observe(data)
```

- [ ] **Step 7: Let the flight loop use it** — in `internal/app/flight.go` `positionLoop`, delete:

```go
	// The locator reads airport layouts on its own goroutine for the same
	// reason: Observe must stay cheap enough for the 33ms flare tick.
	locator := a.startGroundLocator()
	defer locator.Stop()
```

and replace both `locator.Observe(fd)` calls with `a.currentLocator().Observe(fd)`. Above the first one add:

```go
			// The data stream owns the locator, and a reconnect replaces it,
			// so it is asked for per sample. Observe never blocks.
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./internal/app/ -count=1`
Expected: `ok  airspace-acars/internal/app`

- [ ] **Step 9: Commit**

```bash
git add internal/domain/debug.go internal/app/ground_locator.go internal/app/ground_locator_test.go internal/app/app.go internal/app/sim.go internal/app/flight.go
git commit -m "feat(position): look runway and stand up whenever a simulator is connected"
```

---

### Task 2: Report tap, preview and the debug service (Go)

**Files:**
- Create: `internal/app/report_tap.go`, `internal/app/debug.go`
- Modify: `internal/app/position_upload.go` (`send`, `persist`)
- Modify: `internal/app/app.go` (field + `NewApp`)
- Modify: `internal/app/sim.go` (call `observeGround` instead of `locator.Observe`)
- Modify: `internal/app/flight.go` (`buildPositionReport` elapsed time)
- Modify: `services.go`, `main.go`
- Test: `internal/app/report_tap_test.go`

**Interfaces:**
- Consumes: Task 1's `domain.*` types, `groundLocator.Status`, `currentLocator`.
- Produces: `type reportTap` with `record(batch []map[string]interface{}, outcome string)` and `snapshot() domain.PositionReportSnapshot` (both nil-safe); App field `reports *reportTap`; `func (a *App) observeGround(*groundLocator, *domain.FlightData)`; `func (a *App) DebugSnapshot() domain.DebugSnapshot`; Wails service `DebugService.GetDebugSnapshot() domain.DebugSnapshot`.

- [ ] **Step 1: Write the failing tests** — create `internal/app/report_tap_test.go`:

```go
package app

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"airspace-acars/internal/domain"
)

// The debug screen shows the report that went out, not a rebuilt copy.
func TestASentReportIsKeptAsSent(t *testing.T) {
	a := &App{Airspace: &fixedAPI{status: 202}, DB: newMemDB(), UI: nullUI{}, reports: &reportTap{}}
	u := a.startPositionUploader("booking-718")
	u.Submit(samples(3))
	u.Stop()

	s := a.reports.snapshot()
	if s.Outcome != domain.ReportSent || s.BatchSize != 3 {
		t.Fatalf("kept %q from a batch of %d, want sent from 3", s.Outcome, s.BatchSize)
	}
	if !strings.Contains(s.JSON, `"timestamp": 2`) {
		t.Errorf("kept %s, want the batch's last report", s.JSON)
	}
}

// A report that failed is shown as waiting in the outbox; one with no
// booking to be filed under is shown as dropped.
func TestAFailedReportIsKeptWithItsFate(t *testing.T) {
	quickRetries(t)
	for _, tc := range []struct {
		booking, want string
	}{
		{"booking-718", domain.ReportOutbox},
		{"", domain.ReportDropped},
	} {
		a := &App{Airspace: &fixedAPI{status: 502}, DB: newMemDB(), UI: nullUI{}, reports: &reportTap{}}
		u := a.startPositionUploader(tc.booking)
		u.Submit(samples(2))
		u.Stop()
		if s := a.reports.snapshot(); s.Outcome != tc.want {
			t.Errorf("booking %q: outcome %q, want %q", tc.booking, s.Outcome, tc.want)
		}
	}
}

// Before a flight the screen shows the report that would be sent, built by
// the same code, with runway and stand, and no elapsed time.
func TestOutsideAFlightThePreviewIsTheReportThatWouldBeSent(t *testing.T) {
	a := &App{reports: &reportTap{}}
	l := newTestLocator(&fakeLayouts{layout: egll()})
	a.observeGround(l.groundLocator, onRunway09L())
	l.serve()
	a.observeGround(l.groundLocator, onRunway09L())

	s := a.reports.snapshot()
	if s.Outcome != domain.ReportPreview {
		t.Fatalf("outcome %q, want preview", s.Outcome)
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(s.JSON), &got); err != nil {
		t.Fatalf("preview is not JSON: %v\n%s", err, s.JSON)
	}
	if !strings.Contains(s.JSON, `"designator": "09L"`) {
		t.Errorf("preview lacks the runway the aircraft is on:\n%s", s.JSON)
	}
	if v := got["elapsedTime"].(map[string]interface{})["value"]; v != float64(0) {
		t.Errorf("elapsed time before a flight = %v, want 0", v)
	}
}

// During a flight the uploader's report stays on screen: the 1 Hz preview
// must not replace it.
func TestDuringAFlightAPreviewDoesNotReplaceTheSentReport(t *testing.T) {
	a := &App{Airspace: &fixedAPI{status: 202}, DB: newMemDB(), UI: nullUI{}, reports: &reportTap{}, state: "active"}
	u := a.startPositionUploader("booking-718")
	u.Submit(samples(1))
	u.Stop()

	a.observeGround(newTestLocator(&fakeLayouts{}).groundLocator, onRunway09L())
	if s := a.reports.snapshot(); s.Outcome != domain.ReportSent {
		t.Fatalf("outcome %q after a preview tick mid-flight, want sent", s.Outcome)
	}
}

// A reading that is not a number shows as null, as it was sent.
func TestASnapshotEncodesABadReadingAsNull(t *testing.T) {
	tap := &reportTap{}
	tap.record([]map[string]interface{}{{"altitudeAgl": m(math.NaN(), "ft")}}, domain.ReportSent)
	if s := tap.snapshot(); !strings.Contains(s.JSON, `"value": null`) {
		t.Fatalf("NaN reading shown as %s, want null", s.JSON)
	}
	var none *reportTap
	if s := none.snapshot(); s.JSON != "" {
		t.Fatalf("a nil tap returned %+v", s)
	}
}

// One call gives the screen both halves; with nothing connected it is empty,
// not a crash.
func TestTheDebugSnapshotCombinesGroundAndReport(t *testing.T) {
	a := &App{reports: &reportTap{}}
	l := newTestLocator(&fakeLayouts{layout: egll()})
	a.locator = l.groundLocator
	a.observeGround(a.locator, onRunway09L())
	l.serve()
	a.observeGround(a.locator, onRunway09L())

	snap := a.DebugSnapshot()
	if snap.Ground.Runway != "09L/27R" || snap.Report.Outcome != domain.ReportPreview {
		t.Fatalf("snapshot = %+v", snap)
	}
	if empty := (&App{}).DebugSnapshot(); empty.Ground.Airport != "" || empty.Report.JSON != "" {
		t.Fatalf("snapshot with nothing connected = %+v", empty)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/ -run 'Report|Preview|Snapshot' -count=1`
Expected: build failure, `undefined: reportTap`, `a.observeGround undefined`, `a.DebugSnapshot undefined`.

- [ ] **Step 3: Write the tap** — create `internal/app/report_tap.go`:

```go
package app

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"airspace-acars/internal/domain"
)

// reportTap keeps the last position report the uploader handled, and what
// became of it, for the debug screen; before a flight, the report that would
// be sent. It holds the report map itself and encodes it only when asked, so
// a screen nobody is looking at costs nothing.
type reportTap struct {
	mu        sync.Mutex // guards every field below
	report    map[string]interface{}
	at        time.Time
	outcome   string
	batchSize int
}

// record keeps the last report of a batch and its outcome, one of the
// domain.Report* values. A nil tap records nothing.
func (t *reportTap) record(batch []map[string]interface{}, outcome string) {
	if t == nil || len(batch) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.report = batch[len(batch)-1]
	t.at = time.Now()
	t.outcome = outcome
	t.batchSize = len(batch)
}

// snapshot encodes the kept report with the encoder the upload uses. A report
// map is not changed after it is built, so it is encoded outside the lock.
func (t *reportTap) snapshot() domain.PositionReportSnapshot {
	if t == nil {
		return domain.PositionReportSnapshot{}
	}
	t.mu.Lock()
	report, at, outcome, n := t.report, t.at, t.outcome, t.batchSize
	t.mu.Unlock()
	if report == nil {
		return domain.PositionReportSnapshot{}
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		// measurement already turns NaN into null; anything else that will
		// not encode is shown as such rather than hidden.
		raw = []byte(fmt.Sprintf("encode report: %v", err))
	}
	return domain.PositionReportSnapshot{JSON: string(raw), At: at, Outcome: outcome, BatchSize: n}
}
```

- [ ] **Step 4: Tell the tap what the uploader did** — in `internal/app/position_upload.go`:

In `send`, inside `if err == nil {` after `observability.Add("position.reports_sent", ...)`:

```go
			u.app.reports.record(reports, domain.ReportSent)
```

Replace `persist` with:

```go
func (u *positionUploader) persist(reports []map[string]interface{}, reason string) {
	if u.bookingID == "" {
		// Without a booking there is nowhere durable to put these. Say so
		// rather than discarding them silently.
		slog.Warn("position reports dropped: no booking to file them under",
			"count", len(reports), "reason", reason)
		observability.Add("position.reports_dropped", int64(len(reports)))
		u.app.reports.record(reports, domain.ReportDropped)
		return
	}
	queued := 0
	for _, report := range reports {
		raw, err := json.Marshal(report)
		if err != nil {
			slog.Warn("position: could not encode a report for the outbox",
				"error", err, "reason", reason)
			observability.Count("position.reports_dropped")
			continue
		}
		if err := u.app.DB.EnqueuePosition(u.bookingID, raw); err != nil {
			slog.Warn("position: could not queue a report", "error", err, "reason", reason)
			observability.Count("position.reports_dropped")
			continue
		}
		queued++
		observability.Count("position.reports_queued")
		observability.Count("position.outbox_enqueued")
	}
	outcome := domain.ReportOutbox
	if queued == 0 {
		outcome = domain.ReportDropped
	}
	u.app.reports.record(reports, outcome)
}
```

- [ ] **Step 5: Add the field and create the tap** — in `internal/app/app.go`, after the `locator` field from Task 1:

```go
	// reports keeps the last position report for the debug screen. It
	// locks itself.
	reports *reportTap
```

and in `NewApp`'s literal, after `state: "idle",`:

```go
		reports:              &reportTap{},
```

- [ ] **Step 6: Preview and snapshot** — create `internal/app/debug.go`:

```go
package app

import "airspace-acars/internal/domain"

// observeGround places the aircraft once per data stream tick and, outside a
// flight, keeps the report that would be sent. During a flight the uploader
// keeps the real one, which a preview must not replace.
func (a *App) observeGround(locator *groundLocator, fd *domain.FlightData) {
	fix := locator.Observe(fd)
	if a.flightIdle() {
		a.reports.record([]map[string]interface{}{a.buildPositionReport(fd, fix)}, domain.ReportPreview)
	}
}

// flightIdle reports whether no flight is under way.
func (a *App) flightIdle() bool {
	a.flightMu.Lock()
	defer a.flightMu.Unlock()
	return a.state == "" || a.state == "idle"
}

// DebugSnapshot is what the debug screen polls for: where the aircraft is on
// the airport, the last position report and its fate, and the active
// aircraft profiles.
func (a *App) DebugSnapshot() domain.DebugSnapshot {
	snap := domain.DebugSnapshot{
		Ground: a.currentLocator().Status(),
		Report: a.reports.snapshot(),
	}
	if plan := a.GetActiveProfile(); plan != nil {
		snap.Profiles = plan.ProfileIDs()
	}
	return snap
}
```

In `internal/app/sim.go`, replace the Task 1 line `locator.Observe(data)` with:

```go
			a.observeGround(locator, data)
```

In `internal/app/flight.go` `buildPositionReport`, replace `elapsed := time.Since(a.startTime).Seconds()` with:

```go
	elapsed := 0.0
	if !a.startTime.IsZero() {
		// A preview before any flight has no start to count from.
		elapsed = time.Since(a.startTime).Seconds()
	}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/app/ -count=1`
Expected: `ok  airspace-acars/internal/app`

- [ ] **Step 8: Expose the service** — append to `services.go`:

```go
// --- DebugService: the debug tab ---

type DebugService struct{ app *app.App }

func (s *DebugService) GetDebugSnapshot() domain.DebugSnapshot { return s.app.DebugSnapshot() }
```

In `main.go`, after `flightLogSvc := ...` add `debugSvc := &DebugService{app: appInstance}` and after `application.NewService(flightLogSvc),` add `application.NewService(debugSvc),`.

- [ ] **Step 9: Build the root package and regenerate bindings**

Run:
```bash
mkdir -p frontend/dist && touch frontend/dist/.gitkeep
go build . && go vet ./internal/app/ ./internal/domain/
"$(go env GOPATH)/bin/wails3" generate bindings -ts
```
Expected: build and vet print nothing; bindings include `DebugService` (`ls frontend/bindings/airspace-acars/debugservice.ts`).

- [ ] **Step 10: Commit**

```bash
git add internal/app/report_tap.go internal/app/report_tap_test.go internal/app/debug.go internal/app/position_upload.go internal/app/app.go internal/app/sim.go internal/app/flight.go services.go main.go
git commit -m "feat(position): keep the last sent report for the debug screen"
```

---

### Task 3: Snapshot types, formatting and polling hook (frontend)

**Files:**
- Create: `frontend/src/lib/debug-snapshot.ts`, `frontend/src/lib/debug-snapshot.test.ts`
- Create: `frontend/src/hooks/use-debug-snapshot.ts`, `frontend/src/hooks/use-debug-snapshot.test.tsx`
- Modify: `frontend/src/__mocks__/wails-bindings.ts`, `frontend/src/__mocks__/wails-bindings-module.ts`

**Interfaces:**
- Consumes: `DebugService.GetDebugSnapshot()` (Task 2).
- Produces: types `DebugSnapshot`, `GroundStatus`, `PositionReportSnapshot`, `ReportOutcome`; `isUnsetTime(iso)`, `secondsSince(iso, now)`, `outcomeKey(outcome)`; hook `useDebugSnapshot(): DebugSnapshot | null`; constant `DEBUG_POLL_MS = 1000`; mock `DebugService`.

- [ ] **Step 1: Write the failing tests** — create `frontend/src/lib/debug-snapshot.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { isUnsetTime, outcomeKey, secondsSince } from "./debug-snapshot";

describe("debug snapshot helpers", () => {
  it("treats Go's zero time as unset", () => {
    expect(isUnsetTime("0001-01-01T00:00:00Z")).toBe(true);
    expect(isUnsetTime("")).toBe(true);
    expect(isUnsetTime(undefined)).toBe(true);
    expect(isUnsetTime("2026-10-08T03:00:00Z")).toBe(false);
  });

  it("counts whole seconds since a time, and nothing for an unset one", () => {
    const now = Date.parse("2026-10-08T03:00:12Z");
    expect(secondsSince("2026-10-08T03:00:00Z", now)).toBe(12);
    expect(secondsSince("0001-01-01T00:00:00Z", now)).toBeNull();
    expect(secondsSince("not a time", now)).toBeNull();
    expect(secondsSince("2026-10-08T03:00:30Z", now)).toBe(0);
  });

  it("maps every outcome to a translation key", () => {
    expect(outcomeKey("sent")).toBe("debug.outcome.sent");
    expect(outcomeKey("outbox")).toBe("debug.outcome.outbox");
    expect(outcomeKey("dropped")).toBe("debug.outcome.dropped");
    expect(outcomeKey("preview")).toBe("debug.outcome.preview");
    expect(outcomeKey("")).toBe("debug.outcome.none");
  });
});
```

and `frontend/src/hooks/use-debug-snapshot.test.tsx`:

```tsx
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { DebugService } from "../../bindings/airspace-acars";
import { DEBUG_POLL_MS, useDebugSnapshot } from "./use-debug-snapshot";
import { EMPTY_SNAPSHOT } from "@/lib/debug-snapshot";

const snap = { ...EMPTY_SNAPSHOT, ground: { ...EMPTY_SNAPSHOT.ground, airport: "SBRF", stand: "7" } };

beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(DebugService, "GetDebugSnapshot").mockResolvedValue(snap);
});
afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); });

it("polls once a second and returns the latest snapshot", async () => {
  const { result } = renderHook(() => useDebugSnapshot());
  await act(async () => { await Promise.resolve(); });
  expect(result.current?.ground.stand).toBe("7");
  await act(async () => { vi.advanceTimersByTime(DEBUG_POLL_MS * 2); });
  expect(DebugService.GetDebugSnapshot).toHaveBeenCalledTimes(3);
});

it("stops polling when unmounted", async () => {
  const { unmount } = renderHook(() => useDebugSnapshot());
  await act(async () => { await Promise.resolve(); });
  unmount();
  await act(async () => { vi.advanceTimersByTime(DEBUG_POLL_MS * 5); });
  expect(DebugService.GetDebugSnapshot).toHaveBeenCalledTimes(1);
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd frontend && npx vitest run src/lib/debug-snapshot.test.ts src/hooks/use-debug-snapshot.test.tsx`
Expected: FAIL, cannot resolve `./debug-snapshot` and `./use-debug-snapshot`.

- [ ] **Step 3: Write the helpers** — create `frontend/src/lib/debug-snapshot.ts`:

```ts
// The debug screen's view of DebugService.GetDebugSnapshot, typed here as
// use-flight-data.ts types FlightData, so tests need no generated bindings.

export type ReportOutcome = "" | "sent" | "outbox" | "dropped" | "preview";

export interface GroundStatus {
  airport: string;
  runways: number;
  stands: number;
  runway: string;
  stand: string;
  loadedAt: string;
  lastError: string;
}

export interface PositionReportSnapshot {
  json: string;
  at: string;
  outcome: ReportOutcome;
  batchSize: number;
}

export interface DebugSnapshot {
  ground: GroundStatus;
  report: PositionReportSnapshot;
  profiles: string[] | null;
}

const GO_ZERO_TIME = "0001-01-01T00:00:00Z";

export const EMPTY_SNAPSHOT: DebugSnapshot = {
  ground: { airport: "", runways: 0, stands: 0, runway: "", stand: "", loadedAt: GO_ZERO_TIME, lastError: "" },
  report: { json: "", at: GO_ZERO_TIME, outcome: "", batchSize: 0 },
  profiles: null,
};

/** Go encodes an unset time.Time as year 1. */
export function isUnsetTime(iso: string | null | undefined): boolean {
  return !iso || iso.startsWith("0001-01-01");
}

/** Whole seconds from iso to now, or null when the time is unset or unreadable. */
export function secondsSince(iso: string | null | undefined, now: number): number | null {
  if (isUnsetTime(iso)) return null;
  const t = Date.parse(iso as string);
  if (Number.isNaN(t)) return null;
  return Math.max(0, Math.round((now - t) / 1000));
}

const OUTCOME_KEYS: Record<string, string> = {
  sent: "debug.outcome.sent",
  outbox: "debug.outcome.outbox",
  dropped: "debug.outcome.dropped",
  preview: "debug.outcome.preview",
};

/** The translation key for what became of a report. */
export function outcomeKey(outcome: string): string {
  return OUTCOME_KEYS[outcome] ?? "debug.outcome.none";
}
```

- [ ] **Step 4: Write the hook** — create `frontend/src/hooks/use-debug-snapshot.ts`:

```ts
import { useEffect, useState } from "react";
import { DebugService } from "../../bindings/airspace-acars";
import type { DebugSnapshot } from "@/lib/debug-snapshot";

/** How often the debug tab asks for a snapshot: the data stream's own rate. */
export const DEBUG_POLL_MS = 1000;

/** The backend's debug snapshot, refreshed while the calling component is mounted. */
export function useDebugSnapshot(): DebugSnapshot | null {
  const [snapshot, setSnapshot] = useState<DebugSnapshot | null>(null);
  useEffect(() => {
    let cancelled = false;
    const poll = () => {
      Promise.resolve(DebugService.GetDebugSnapshot())
        .then((s) => {
          if (!cancelled) setSnapshot(s as unknown as DebugSnapshot);
        })
        // A missed poll is retried a second later; there is nothing to show.
        .catch(() => {});
    };
    poll();
    const id = setInterval(poll, DEBUG_POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);
  return snapshot;
}
```

- [ ] **Step 5: Mock the service** — in `frontend/src/__mocks__/wails-bindings.ts` add:

```ts
export function mockDebugService() {
  return {
    GetDebugSnapshot: () =>
      Promise.resolve({
        ground: { airport: "", runways: 0, stands: 0, runway: "", stand: "", loadedAt: "0001-01-01T00:00:00Z", lastError: "" },
        report: { json: "", at: "0001-01-01T00:00:00Z", outcome: "", batchSize: 0 },
        profiles: null,
      }),
  };
}
```

In `frontend/src/__mocks__/wails-bindings-module.ts` add `mockDebugService,` to the import list and `export const DebugService = mockDebugService();` beside the others.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd frontend && npx vitest run src/lib/debug-snapshot.test.ts src/hooks/use-debug-snapshot.test.tsx`
Expected: PASS (5 tests).

- [ ] **Step 7: Commit**

```bash
git add frontend/src/lib/debug-snapshot.ts frontend/src/lib/debug-snapshot.test.ts frontend/src/hooks/use-debug-snapshot.ts frontend/src/hooks/use-debug-snapshot.test.tsx frontend/src/__mocks__/wails-bindings.ts frontend/src/__mocks__/wails-bindings-module.ts
git commit -m "feat(frontend): poll the debug snapshot while the debug tab is open"
```

---

### Task 4: Reorganised debug tab (frontend)

**Files:**
- Create: `frontend/src/components/debug/debug-table.tsx`, `debug-status-strip.tsx`, `debug-overview.tsx`, `debug-telemetry.tsx`, `debug-payload.tsx`, `debug-logs.tsx` (all under `frontend/src/components/debug/`)
- Modify (rewrite): `frontend/src/components/debug-tab.tsx`
- Modify: `frontend/src/locales/{en,pt,es,fr}.json`, `frontend/src/locales/locale-parity.test.ts`
- Test: `frontend/src/components/debug-tab.test.tsx`

**Interfaces:**
- Consumes: `useDebugSnapshot`, `DebugSnapshot`, `secondsSince`, `isUnsetTime`, `outcomeKey` (Task 3); `useFlightData()` returning `{ flightData: FlightData | null }` (existing, `src/hooks/use-flight-data.ts`); `FlightDataService.IsConnected/ConnectedAdapter`, `UpdateService.TailLogs`.
- Produces: `DebugTab` (same export name and import path as today, so `app-shell.tsx` is unchanged).

- [ ] **Step 1: Write the failing tests** — create `frontend/src/components/debug-tab.test.tsx`:

```tsx
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { DebugService, FlightDataService } from "../../bindings/airspace-acars";
import { EMPTY_SNAPSHOT, type DebugSnapshot } from "@/lib/debug-snapshot";
import en from "@/locales/en.json";
import { DebugTab } from "./debug-tab";

vi.mock("@/hooks/use-flight-data", () => ({ useFlightData: () => ({ flightData: null }) }));
vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => {} } }));

const i18n = createInstance();
await i18n.init({ lng: "en", resources: { en: { translation: en } }, keySeparator: false, interpolation: { escapeValue: false } });

const sentAt = new Date(Date.now() - 4000).toISOString();
const atGate7: DebugSnapshot = {
  ground: { airport: "SBRF", runways: 1, stands: 41, runway: "", stand: "7", loadedAt: sentAt, lastError: "" },
  report: { json: '{\n  "stand": {\n    "name": "7"\n  }\n}', at: sentAt, outcome: "preview", batchSize: 1 },
  profiles: ["fenix-a32x"],
};

function view() {
  return <I18nextProvider i18n={i18n}><DebugTab /></I18nextProvider>;
}

beforeEach(() => {
  vi.spyOn(DebugService, "GetDebugSnapshot").mockResolvedValue(atGate7);
  vi.spyOn(FlightDataService, "IsConnected").mockResolvedValue(true);
  vi.spyOn(FlightDataService, "ConnectedAdapter").mockResolvedValue("SimConnect");
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

it("shows the airport and the stand the backend places the aircraft on", async () => {
  render(view());
  expect(await screen.findByRole("row", { name: /Stand\s*7/ })).toBeInTheDocument();
  expect(screen.getByRole("row", { name: /Airport\s*SBRF · 1 runways, 41 stands/ })).toBeInTheDocument();
  expect(screen.getByRole("row", { name: /Runway\s*—/ })).toBeInTheDocument();
});

it("says so when no airport is near", async () => {
  vi.mocked(DebugService.GetDebugSnapshot).mockResolvedValue(EMPTY_SNAPSHOT);
  render(view());
  expect(await screen.findByText(en["debug.ground.noAirport"])).toBeInTheDocument();
  expect(screen.getByText(en["debug.ground.notLoaded"])).toBeInTheDocument();
});

it("shows the connection, adapter, profile and last report in the status strip", async () => {
  render(view());
  const strip = await screen.findByRole("status");
  expect(await within(strip).findByText(en["debug.connected"])).toBeInTheDocument();
  expect(within(strip).getByText("SimConnect")).toBeInTheDocument();
  expect(within(strip).getByText("Profile: fenix-a32x")).toBeInTheDocument();
  expect(within(strip).getByText(/Last report \d+ s ago · preview/)).toBeInTheDocument();
});

it("shows the report exactly as built, marked as a preview, and copies it", async () => {
  const user = userEvent.setup();
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
  render(view());
  await user.click(await screen.findByRole("tab", { name: en["debug.tab.payload"] }));

  expect(await screen.findByText(/"name": "7"/)).toBeInTheDocument();
  expect(screen.getByText(en["debug.payload.previewNote"])).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: en["debug.copyJson"] }));
  expect(writeText).toHaveBeenCalledWith(atGate7.report.json);
});

it("copy does nothing without a clipboard", async () => {
  const user = userEvent.setup();
  Object.defineProperty(navigator, "clipboard", { value: undefined, configurable: true });
  render(view());
  await user.click(await screen.findByRole("tab", { name: en["debug.tab.payload"] }));
  await user.click(await screen.findByRole("button", { name: en["debug.copyJson"] }));
  expect(screen.getByRole("button", { name: en["debug.copyJson"] })).toBeInTheDocument();
});
```

Add to `frontend/src/locales/locale-parity.test.ts`, inside the `describe`, beside the `myFlights.` test:

```ts
  it("ensures all debug keys in en.json exist in pt, es, and fr", () => {
    const enDebugKeys = Object.keys(en).filter((k) => k.startsWith("debug."));
    expect(enDebugKeys.length).toBeGreaterThan(0);
    for (const locale of locales) {
      const missingKeys = enDebugKeys.filter((k) => !(k in locale.data));
      expect(missingKeys, `${locale.name}.json is missing debug keys`).toEqual([]);
    }
  });
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd frontend && npx vitest run src/components/debug-tab.test.tsx src/locales/locale-parity.test.ts`
Expected: FAIL — no `row` named `Stand 7`, no `tab` role, and the parity test lists the missing `debug.*` keys once Step 3 adds them to `en.json` only.

- [ ] **Step 3: Add the strings** — in each locale file, delete `"debug.updates"` and `"debug.apiPayload"` (first `grep -rn 'debug.updates\|debug.apiPayload' frontend/src` must show only `debug-tab.tsx`), keep the other existing `debug.*` keys, and add these beside them.

`en.json`:
```json
  "debug.tab.overview": "Overview",
  "debug.tab.telemetry": "Telemetry",
  "debug.tab.payload": "Payload",
  "debug.tab.logs": "Logs",
  "debug.logs": "Application logs",
  "debug.logsPause": "Pause",
  "debug.logsResume": "Resume",
  "debug.logsRefresh": "Refresh",
  "debug.logsEmpty": "No logs available",
  "debug.strip.noAircraft": "No aircraft",
  "debug.strip.noProfile": "No aircraft profile",
  "debug.strip.profile": "Profile: {{name}}",
  "debug.strip.onGround": "On ground",
  "debug.strip.airborne": "Airborne",
  "debug.strip.noReport": "No report yet",
  "debug.strip.report": "Last report {{age}} s ago · {{outcome}}",
  "debug.ground.title": "Ground position",
  "debug.ground.airport": "Airport",
  "debug.ground.runway": "Runway",
  "debug.ground.stand": "Stand",
  "debug.ground.lookup": "Lookup",
  "debug.ground.noAirport": "No airport nearby",
  "debug.ground.counts": "{{runways}} runways, {{stands}} stands",
  "debug.ground.notLoaded": "Not loaded yet",
  "debug.ground.loadedAgo": "Loaded {{age}} s ago",
  "debug.ground.error": "Last lookup failed: {{error}}",
  "debug.outcome.sent": "sent",
  "debug.outcome.outbox": "queued in the outbox",
  "debug.outcome.dropped": "dropped",
  "debug.outcome.preview": "preview",
  "debug.outcome.none": "unknown",
  "debug.payload.title": "Last position report",
  "debug.payload.empty": "No position report yet. Connect a simulator to see one.",
  "debug.payload.meta": "{{outcome}} · {{age}} s ago · batch of {{count}}",
  "debug.payload.previewNote": "Not sent. This is the report the ACARS would send now; reports are only sent during a flight.",
  "debug.group.flight": "Flight",
  "debug.group.systems": "Engines & systems",
  "debug.group.radios": "Radios & autopilot",
  "debug.group.lightsControls": "Lights & controls",
  "debug.row.latitude": "Latitude",
  "debug.row.longitude": "Longitude",
  "debug.row.altitude": "Altitude",
  "debug.row.agl": "AGL",
  "debug.row.pitch": "Pitch",
  "debug.row.roll": "Roll",
  "debug.row.headingTrue": "Heading (true)",
  "debug.row.headingMag": "Heading (magnetic)",
  "debug.row.vs": "Vertical speed",
  "debug.row.ias": "IAS",
  "debug.row.tas": "TAS",
  "debug.row.gs": "Ground speed",
  "debug.row.onGround": "On ground",
  "debug.row.stall": "Stall warning",
  "debug.row.overspeed": "Overspeed",
  "debug.row.beacon": "Beacon",
  "debug.row.strobe": "Strobe",
  "debug.row.landing": "Landing",
  "debug.row.engine": "Eng",
  "debug.row.running": "Run",
  "debug.row.n1": "N1 %",
  "debug.row.n2": "N2 %",
  "debug.row.throttle": "Thr %",
  "debug.row.mixture": "Mix %",
  "debug.row.prop": "Prop %",
  "debug.row.com1": "COM1",
  "debug.row.com2": "COM2",
  "debug.row.nav1": "NAV1",
  "debug.row.nav2": "NAV2",
  "debug.row.nav1Obs": "NAV1 OBS",
  "debug.row.nav2Obs": "NAV2 OBS",
  "debug.row.xpdrCode": "XPDR code",
  "debug.row.xpdrState": "XPDR mode",
  "debug.row.apMaster": "Engaged",
  "debug.row.heading": "Heading",
  "debug.row.speed": "Speed",
  "debug.row.approach": "Approach",
  "debug.row.navLock": "NAV lock",
  "debug.row.elevator": "Elevator",
  "debug.row.aileron": "Aileron",
  "debug.row.rudder": "Rudder",
  "debug.row.flaps": "Flaps",
  "debug.row.spoilers": "Spoilers",
  "debug.row.gearDown": "Gear down",
  "debug.row.apuSwitch": "Switch",
  "debug.row.apuRpm": "RPM",
  "debug.row.genSwitch": "Generator switch",
  "debug.row.genActive": "Generator online",
  "debug.row.door": "Door {{n}}",
  "debug.row.total": "Total",
  "debug.row.fuel": "Fuel",
  "debug.row.aircraft": "Aircraft",
  "debug.row.altimeter": "Altimeter",
  "debug.row.zulu": "Zulu time",
  "debug.row.local": "Local time",
```

`pt.json`:
```json
  "debug.tab.overview": "Visão geral",
  "debug.tab.telemetry": "Telemetria",
  "debug.tab.payload": "Payload",
  "debug.tab.logs": "Logs",
  "debug.logs": "Logs do aplicativo",
  "debug.logsPause": "Pausar",
  "debug.logsResume": "Retomar",
  "debug.logsRefresh": "Atualizar",
  "debug.logsEmpty": "Nenhum log disponível",
  "debug.strip.noAircraft": "Nenhuma aeronave",
  "debug.strip.noProfile": "Sem perfil de aeronave",
  "debug.strip.profile": "Perfil: {{name}}",
  "debug.strip.onGround": "No solo",
  "debug.strip.airborne": "Em voo",
  "debug.strip.noReport": "Nenhum relatório ainda",
  "debug.strip.report": "Último relatório há {{age}} s · {{outcome}}",
  "debug.ground.title": "Posição no solo",
  "debug.ground.airport": "Aeroporto",
  "debug.ground.runway": "Pista",
  "debug.ground.stand": "Posição",
  "debug.ground.lookup": "Consulta",
  "debug.ground.noAirport": "Nenhum aeroporto próximo",
  "debug.ground.counts": "{{runways}} pistas, {{stands}} posições",
  "debug.ground.notLoaded": "Ainda não carregado",
  "debug.ground.loadedAgo": "Carregado há {{age}} s",
  "debug.ground.error": "A última consulta falhou: {{error}}",
  "debug.outcome.sent": "enviado",
  "debug.outcome.outbox": "na fila de envio",
  "debug.outcome.dropped": "descartado",
  "debug.outcome.preview": "prévia",
  "debug.outcome.none": "desconhecido",
  "debug.payload.title": "Último relatório de posição",
  "debug.payload.empty": "Nenhum relatório de posição ainda. Conecte um simulador para ver um.",
  "debug.payload.meta": "{{outcome}} · há {{age}} s · lote de {{count}}",
  "debug.payload.previewNote": "Não enviado. Este é o relatório que o ACARS enviaria agora; relatórios só são enviados durante um voo.",
  "debug.group.flight": "Voo",
  "debug.group.systems": "Motores e sistemas",
  "debug.group.radios": "Rádios e piloto automático",
  "debug.group.lightsControls": "Luzes e comandos",
  "debug.row.latitude": "Latitude",
  "debug.row.longitude": "Longitude",
  "debug.row.altitude": "Altitude",
  "debug.row.agl": "AGL",
  "debug.row.pitch": "Arfagem",
  "debug.row.roll": "Rolagem",
  "debug.row.headingTrue": "Proa (verdadeira)",
  "debug.row.headingMag": "Proa (magnética)",
  "debug.row.vs": "Velocidade vertical",
  "debug.row.ias": "IAS",
  "debug.row.tas": "TAS",
  "debug.row.gs": "Velocidade no solo",
  "debug.row.onGround": "No solo",
  "debug.row.stall": "Alerta de estol",
  "debug.row.overspeed": "Sobrevelocidade",
  "debug.row.beacon": "Beacon",
  "debug.row.strobe": "Strobe",
  "debug.row.landing": "Pouso",
  "debug.row.engine": "Mot",
  "debug.row.running": "Lig",
  "debug.row.n1": "N1 %",
  "debug.row.n2": "N2 %",
  "debug.row.throttle": "Man %",
  "debug.row.mixture": "Mist %",
  "debug.row.prop": "Hél %",
  "debug.row.com1": "COM1",
  "debug.row.com2": "COM2",
  "debug.row.nav1": "NAV1",
  "debug.row.nav2": "NAV2",
  "debug.row.nav1Obs": "NAV1 OBS",
  "debug.row.nav2Obs": "NAV2 OBS",
  "debug.row.xpdrCode": "Código XPDR",
  "debug.row.xpdrState": "Modo XPDR",
  "debug.row.apMaster": "Engajado",
  "debug.row.heading": "Proa",
  "debug.row.speed": "Velocidade",
  "debug.row.approach": "Aproximação",
  "debug.row.navLock": "NAV travado",
  "debug.row.elevator": "Profundor",
  "debug.row.aileron": "Aileron",
  "debug.row.rudder": "Leme",
  "debug.row.flaps": "Flapes",
  "debug.row.spoilers": "Spoilers",
  "debug.row.gearDown": "Trem baixado",
  "debug.row.apuSwitch": "Chave",
  "debug.row.apuRpm": "RPM",
  "debug.row.genSwitch": "Chave do gerador",
  "debug.row.genActive": "Gerador ativo",
  "debug.row.door": "Porta {{n}}",
  "debug.row.total": "Total",
  "debug.row.fuel": "Combustível",
  "debug.row.aircraft": "Aeronave",
  "debug.row.altimeter": "Altímetro",
  "debug.row.zulu": "Hora Zulu",
  "debug.row.local": "Hora local",
```

`es.json`:
```json
  "debug.tab.overview": "Resumen",
  "debug.tab.telemetry": "Telemetría",
  "debug.tab.payload": "Payload",
  "debug.tab.logs": "Registros",
  "debug.logs": "Registros de la aplicación",
  "debug.logsPause": "Pausar",
  "debug.logsResume": "Reanudar",
  "debug.logsRefresh": "Actualizar",
  "debug.logsEmpty": "No hay registros disponibles",
  "debug.strip.noAircraft": "Sin aeronave",
  "debug.strip.noProfile": "Sin perfil de aeronave",
  "debug.strip.profile": "Perfil: {{name}}",
  "debug.strip.onGround": "En tierra",
  "debug.strip.airborne": "En vuelo",
  "debug.strip.noReport": "Aún no hay reporte",
  "debug.strip.report": "Último reporte hace {{age}} s · {{outcome}}",
  "debug.ground.title": "Posición en tierra",
  "debug.ground.airport": "Aeropuerto",
  "debug.ground.runway": "Pista",
  "debug.ground.stand": "Puesto",
  "debug.ground.lookup": "Consulta",
  "debug.ground.noAirport": "Ningún aeropuerto cercano",
  "debug.ground.counts": "{{runways}} pistas, {{stands}} puestos",
  "debug.ground.notLoaded": "Aún no cargado",
  "debug.ground.loadedAgo": "Cargado hace {{age}} s",
  "debug.ground.error": "La última consulta falló: {{error}}",
  "debug.outcome.sent": "enviado",
  "debug.outcome.outbox": "en la cola de envío",
  "debug.outcome.dropped": "descartado",
  "debug.outcome.preview": "vista previa",
  "debug.outcome.none": "desconocido",
  "debug.payload.title": "Último reporte de posición",
  "debug.payload.empty": "Aún no hay reporte de posición. Conecta un simulador para ver uno.",
  "debug.payload.meta": "{{outcome}} · hace {{age}} s · lote de {{count}}",
  "debug.payload.previewNote": "No enviado. Este es el reporte que el ACARS enviaría ahora; los reportes solo se envían durante un vuelo.",
  "debug.group.flight": "Vuelo",
  "debug.group.systems": "Motores y sistemas",
  "debug.group.radios": "Radios y piloto automático",
  "debug.group.lightsControls": "Luces y mandos",
  "debug.row.latitude": "Latitud",
  "debug.row.longitude": "Longitud",
  "debug.row.altitude": "Altitud",
  "debug.row.agl": "AGL",
  "debug.row.pitch": "Cabeceo",
  "debug.row.roll": "Alabeo",
  "debug.row.headingTrue": "Rumbo (verdadero)",
  "debug.row.headingMag": "Rumbo (magnético)",
  "debug.row.vs": "Velocidad vertical",
  "debug.row.ias": "IAS",
  "debug.row.tas": "TAS",
  "debug.row.gs": "Velocidad sobre el suelo",
  "debug.row.onGround": "En tierra",
  "debug.row.stall": "Alerta de pérdida",
  "debug.row.overspeed": "Sobrevelocidad",
  "debug.row.beacon": "Baliza",
  "debug.row.strobe": "Estroboscópicas",
  "debug.row.landing": "Aterrizaje",
  "debug.row.engine": "Mot",
  "debug.row.running": "Enc",
  "debug.row.n1": "N1 %",
  "debug.row.n2": "N2 %",
  "debug.row.throttle": "Pot %",
  "debug.row.mixture": "Mezcla %",
  "debug.row.prop": "Hél %",
  "debug.row.com1": "COM1",
  "debug.row.com2": "COM2",
  "debug.row.nav1": "NAV1",
  "debug.row.nav2": "NAV2",
  "debug.row.nav1Obs": "NAV1 OBS",
  "debug.row.nav2Obs": "NAV2 OBS",
  "debug.row.xpdrCode": "Código XPDR",
  "debug.row.xpdrState": "Modo XPDR",
  "debug.row.apMaster": "Conectado",
  "debug.row.heading": "Rumbo",
  "debug.row.speed": "Velocidad",
  "debug.row.approach": "Aproximación",
  "debug.row.navLock": "NAV bloqueado",
  "debug.row.elevator": "Timón de profundidad",
  "debug.row.aileron": "Alerón",
  "debug.row.rudder": "Timón de dirección",
  "debug.row.flaps": "Flaps",
  "debug.row.spoilers": "Spoilers",
  "debug.row.gearDown": "Tren abajo",
  "debug.row.apuSwitch": "Interruptor",
  "debug.row.apuRpm": "RPM",
  "debug.row.genSwitch": "Interruptor del generador",
  "debug.row.genActive": "Generador activo",
  "debug.row.door": "Puerta {{n}}",
  "debug.row.total": "Total",
  "debug.row.fuel": "Combustible",
  "debug.row.aircraft": "Aeronave",
  "debug.row.altimeter": "Altímetro",
  "debug.row.zulu": "Hora Zulu",
  "debug.row.local": "Hora local",
```

`fr.json`:
```json
  "debug.tab.overview": "Aperçu",
  "debug.tab.telemetry": "Télémétrie",
  "debug.tab.payload": "Payload",
  "debug.tab.logs": "Journaux",
  "debug.logs": "Journaux de l'application",
  "debug.logsPause": "Pause",
  "debug.logsResume": "Reprendre",
  "debug.logsRefresh": "Actualiser",
  "debug.logsEmpty": "Aucun journal disponible",
  "debug.strip.noAircraft": "Aucun avion",
  "debug.strip.noProfile": "Aucun profil d'avion",
  "debug.strip.profile": "Profil : {{name}}",
  "debug.strip.onGround": "Au sol",
  "debug.strip.airborne": "En vol",
  "debug.strip.noReport": "Aucun rapport pour l'instant",
  "debug.strip.report": "Dernier rapport il y a {{age}} s · {{outcome}}",
  "debug.ground.title": "Position au sol",
  "debug.ground.airport": "Aéroport",
  "debug.ground.runway": "Piste",
  "debug.ground.stand": "Poste",
  "debug.ground.lookup": "Recherche",
  "debug.ground.noAirport": "Aucun aéroport à proximité",
  "debug.ground.counts": "{{runways}} pistes, {{stands}} postes",
  "debug.ground.notLoaded": "Pas encore chargé",
  "debug.ground.loadedAgo": "Chargé il y a {{age}} s",
  "debug.ground.error": "La dernière recherche a échoué : {{error}}",
  "debug.outcome.sent": "envoyé",
  "debug.outcome.outbox": "en file d'attente",
  "debug.outcome.dropped": "abandonné",
  "debug.outcome.preview": "aperçu",
  "debug.outcome.none": "inconnu",
  "debug.payload.title": "Dernier rapport de position",
  "debug.payload.empty": "Aucun rapport de position pour l'instant. Connectez un simulateur pour en voir un.",
  "debug.payload.meta": "{{outcome}} · il y a {{age}} s · lot de {{count}}",
  "debug.payload.previewNote": "Non envoyé. Voici le rapport que l'ACARS enverrait maintenant ; les rapports ne sont envoyés que pendant un vol.",
  "debug.group.flight": "Vol",
  "debug.group.systems": "Moteurs et systèmes",
  "debug.group.radios": "Radios et pilote automatique",
  "debug.group.lightsControls": "Feux et commandes",
  "debug.row.latitude": "Latitude",
  "debug.row.longitude": "Longitude",
  "debug.row.altitude": "Altitude",
  "debug.row.agl": "AGL",
  "debug.row.pitch": "Tangage",
  "debug.row.roll": "Roulis",
  "debug.row.headingTrue": "Cap (vrai)",
  "debug.row.headingMag": "Cap (magnétique)",
  "debug.row.vs": "Vitesse verticale",
  "debug.row.ias": "IAS",
  "debug.row.tas": "TAS",
  "debug.row.gs": "Vitesse sol",
  "debug.row.onGround": "Au sol",
  "debug.row.stall": "Alarme de décrochage",
  "debug.row.overspeed": "Survitesse",
  "debug.row.beacon": "Anticollision",
  "debug.row.strobe": "Strobes",
  "debug.row.landing": "Atterrissage",
  "debug.row.engine": "Mot",
  "debug.row.running": "Marche",
  "debug.row.n1": "N1 %",
  "debug.row.n2": "N2 %",
  "debug.row.throttle": "Gaz %",
  "debug.row.mixture": "Mél %",
  "debug.row.prop": "Hél %",
  "debug.row.com1": "COM1",
  "debug.row.com2": "COM2",
  "debug.row.nav1": "NAV1",
  "debug.row.nav2": "NAV2",
  "debug.row.nav1Obs": "NAV1 OBS",
  "debug.row.nav2Obs": "NAV2 OBS",
  "debug.row.xpdrCode": "Code XPDR",
  "debug.row.xpdrState": "Mode XPDR",
  "debug.row.apMaster": "Engagé",
  "debug.row.heading": "Cap",
  "debug.row.speed": "Vitesse",
  "debug.row.approach": "Approche",
  "debug.row.navLock": "NAV verrouillé",
  "debug.row.elevator": "Profondeur",
  "debug.row.aileron": "Aileron",
  "debug.row.rudder": "Direction",
  "debug.row.flaps": "Volets",
  "debug.row.spoilers": "Spoilers",
  "debug.row.gearDown": "Train sorti",
  "debug.row.apuSwitch": "Interrupteur",
  "debug.row.apuRpm": "RPM",
  "debug.row.genSwitch": "Interrupteur génératrice",
  "debug.row.genActive": "Génératrice active",
  "debug.row.door": "Porte {{n}}",
  "debug.row.total": "Total",
  "debug.row.fuel": "Carburant",
  "debug.row.aircraft": "Avion",
  "debug.row.altimeter": "Altimètre",
  "debug.row.zulu": "Heure Zulu",
  "debug.row.local": "Heure locale",
```

- [ ] **Step 4: Shared table pieces** — create `frontend/src/components/debug/debug-table.tsx`, moving `BoolBadge`, `DataTable` and `fmt` out of today's `debug-tab.tsx` (lines 139–168) and exporting them:

```tsx
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";

export interface DataRow {
  label: string;
  value: ReactNode;
  unit?: string;
}

export function BoolBadge({ value }: { value: boolean }) {
  const { t } = useTranslation();
  return (
    <Badge variant={value ? "default" : "secondary"} className="text-[10px] px-1.5 py-0">
      {value ? t("debug.on") : t("debug.off")}
    </Badge>
  );
}

export function DataTable({ rows }: { rows: DataRow[] }) {
  return (
    <div className="rounded-md border border-border">
      <table className="w-full text-sm">
        <tbody>
          {rows.map((r) => (
            <tr key={r.label} className="border-b border-border/50 last:border-0">
              <td className="px-3 py-1 font-mono text-xs text-muted-foreground w-[160px]">{r.label}</td>
              <td className="px-3 py-1 text-right font-mono text-xs tabular-nums">{r.value}</td>
              {r.unit && <td className="px-2 py-1 text-xs text-muted-foreground w-[50px]">{r.unit}</td>}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function fmt(v: number, d = 2): string {
  return v.toFixed(d);
}

export function SectionTitle({ children }: { children: ReactNode }) {
  return <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{children}</h3>;
}
```

- [ ] **Step 5: Logs** — create `frontend/src/components/debug/debug-logs.tsx` by moving today's `debug-tab.tsx` lines 1–137 that define `tailLogs`, `LogEntry`, `parseLogLine`, `formatTime` and `LogViewer` unchanged, exporting `LogViewer` as `DebugLogs`, with its imports reduced to what those use (`useState, useEffect, useRef, useCallback`, `useTranslation`, `Button`, `UpdateService`). Replace every `t("debug.x", "Default")` with `t("debug.x")`, since the keys now exist in all four locales.

- [ ] **Step 6: Status strip** — create `frontend/src/components/debug/debug-status-strip.tsx`:

```tsx
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import type { FlightData } from "@/hooks/use-flight-data";
import { outcomeKey, secondsSince, type DebugSnapshot } from "@/lib/debug-snapshot";

interface Props {
  connected: boolean;
  adapter: string;
  flightData: FlightData | null;
  snapshot: DebugSnapshot | null;
  now: number;
}

/** One glance at the link, the aircraft and the last report, above every tab. */
export function DebugStatusStrip({ connected, adapter, flightData, snapshot, now }: Props) {
  const { t } = useTranslation();
  const profiles = snapshot?.profiles ?? [];
  const ground = snapshot?.ground;
  const report = snapshot?.report;
  const age = secondsSince(report?.at, now);
  const place = [ground?.airport, ground?.runway, ground?.stand && `${t("debug.ground.stand")} ${ground.stand}`]
    .filter(Boolean)
    .join(" · ");

  return (
    <div role="status" className="space-y-1 rounded-md border border-border bg-muted/30 px-3 py-2 text-xs">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <Badge variant={connected ? "default" : "secondary"}>
          {connected ? t("debug.connected") : t("debug.disconnected")}
        </Badge>
        {adapter && <span>{adapter}</span>}
        <span>·</span>
        <span>{flightData?.aircraftName || t("debug.strip.noAircraft")}</span>
        <span>·</span>
        <span>{profiles.length ? t("debug.strip.profile", { name: profiles.join(", ") }) : t("debug.strip.noProfile")}</span>
      </div>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground">
        {flightData && <span>{flightData.sensors.onGround ? t("debug.strip.onGround") : t("debug.strip.airborne")}</span>}
        {place && <span>{place}</span>}
        <span>
          {age === null || !report?.json
            ? t("debug.strip.noReport")
            : t("debug.strip.report", { age, outcome: t(outcomeKey(report.outcome)) })}
        </span>
      </div>
    </div>
  );
}
```

- [ ] **Step 7: Overview** — create `frontend/src/components/debug/debug-overview.tsx`:

```tsx
import { useTranslation } from "react-i18next";
import type { FlightData } from "@/hooks/use-flight-data";
import { isUnsetTime, secondsSince, type DebugSnapshot } from "@/lib/debug-snapshot";
import { BoolBadge, DataTable, SectionTitle, fmt } from "./debug-table";

interface Props {
  snapshot: DebugSnapshot | null;
  flightData: FlightData | null;
  now: number;
}

/** Where the ACARS places the aircraft, and the few readings that decide it. */
export function DebugOverview({ snapshot, flightData: d, now }: Props) {
  const { t } = useTranslation();
  const g = snapshot?.ground;

  let lookup = t("debug.ground.notLoaded");
  if (g?.lastError) lookup = t("debug.ground.error", { error: g.lastError });
  else if (g && !isUnsetTime(g.loadedAt)) lookup = t("debug.ground.loadedAgo", { age: secondsSince(g.loadedAt, now) });

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <section className="space-y-2">
        <SectionTitle>{t("debug.ground.title")}</SectionTitle>
        <DataTable
          rows={[
            {
              label: t("debug.ground.airport"),
              value: g?.airport
                ? `${g.airport} · ${t("debug.ground.counts", { runways: g.runways, stands: g.stands })}`
                : t("debug.ground.noAirport"),
            },
            { label: t("debug.ground.runway"), value: g?.runway || "—" },
            { label: t("debug.ground.stand"), value: g?.stand || "—" },
            { label: t("debug.ground.lookup"), value: lookup },
          ]}
        />
      </section>
      {d && (
        <section className="space-y-2">
          <SectionTitle>{t("debug.position")}</SectionTitle>
          <DataTable
            rows={[
              { label: t("debug.row.latitude"), value: fmt(d.position.latitude, 6), unit: "deg" },
              { label: t("debug.row.longitude"), value: fmt(d.position.longitude, 6), unit: "deg" },
              { label: t("debug.row.agl"), value: fmt(d.position.altitudeAGL, 0), unit: "ft" },
              { label: t("debug.row.headingTrue"), value: fmt(d.attitude.headingTrue, 1), unit: "deg" },
              { label: t("debug.row.gs"), value: fmt(d.attitude.gs, 1), unit: "kts" },
              { label: t("debug.row.onGround"), value: <BoolBadge value={d.sensors.onGround} /> },
            ]}
          />
        </section>
      )}
    </div>
  );
}
```

- [ ] **Step 8: Payload** — create `frontend/src/components/debug/debug-payload.tsx`:

```tsx
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { outcomeKey, secondsSince, type PositionReportSnapshot } from "@/lib/debug-snapshot";
import { SectionTitle } from "./debug-table";

/** The last position report exactly as the backend built it, and its fate. */
export function DebugPayload({ report, now }: { report: PositionReportSnapshot | undefined; now: number }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  if (!report?.json) {
    return <p className="text-sm text-muted-foreground">{t("debug.payload.empty")}</p>;
  }

  function handleCopy() {
    const clipboard = navigator.clipboard;
    if (!clipboard || !report) return;
    clipboard.writeText(report.json).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }).catch(() => {});
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <div>
          <SectionTitle>{t("debug.payload.title")}</SectionTitle>
          <p className="text-xs text-muted-foreground">
            {t("debug.payload.meta", {
              outcome: t(outcomeKey(report.outcome)),
              age: secondsSince(report.at, now) ?? 0,
              count: report.batchSize,
            })}
          </p>
        </div>
        <Button variant="outline" size="sm" className="h-7 text-xs" onClick={handleCopy}>
          {copied ? t("debug.copied") : t("debug.copyJson")}
        </Button>
      </div>
      {report.outcome === "preview" && (
        <p className="text-xs text-muted-foreground">{t("debug.payload.previewNote")}</p>
      )}
      <pre className="max-h-[480px] overflow-auto rounded-md border border-border bg-muted/50 p-3 font-mono text-[11px] leading-relaxed">
        {report.json}
      </pre>
    </div>
  );
}
```

- [ ] **Step 9: Telemetry** — create `frontend/src/components/debug/debug-telemetry.tsx`. It holds today's tables (`debug-tab.tsx` lines 328–457), regrouped and with every label through `t`:

```tsx
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { FlightData } from "@/hooks/use-flight-data";
import { BoolBadge, DataTable, SectionTitle, fmt } from "./debug-table";

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-3 rounded-md border border-border p-3">
      <h3 className="text-sm font-semibold">{title}</h3>
      {children}
    </section>
  );
}

/** Every reading the simulator gives, grouped the way a pilot thinks of them. */
export function DebugTelemetry({ flightData: d }: { flightData: FlightData }) {
  const { t } = useTranslation();
  const th = "px-2 py-1 text-right text-[10px] font-medium text-muted-foreground";
  const td = "px-2 py-1 text-right font-mono text-xs tabular-nums";

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Group title={t("debug.group.flight")}>
        <SectionTitle>{t("debug.position")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.latitude"), value: fmt(d.position.latitude, 6), unit: "deg" },
          { label: t("debug.row.longitude"), value: fmt(d.position.longitude, 6), unit: "deg" },
          { label: t("debug.row.altitude"), value: fmt(d.position.altitude, 0), unit: "ft" },
          { label: t("debug.row.agl"), value: fmt(d.position.altitudeAGL, 0), unit: "ft" },
        ]} />
        <SectionTitle>{t("debug.attitudeSpeed")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.pitch"), value: fmt(d.attitude.pitch), unit: "deg" },
          { label: t("debug.row.roll"), value: fmt(d.attitude.roll), unit: "deg" },
          { label: t("debug.row.headingTrue"), value: fmt(d.attitude.headingTrue, 1), unit: "deg" },
          { label: t("debug.row.headingMag"), value: fmt(d.attitude.headingMag, 1), unit: "deg" },
          { label: t("debug.row.vs"), value: fmt(d.attitude.vs, 0), unit: "fpm" },
          { label: t("debug.row.ias"), value: fmt(d.attitude.ias, 1), unit: "kts" },
          { label: t("debug.row.tas"), value: fmt(d.attitude.tas, 1), unit: "kts" },
          { label: t("debug.row.gs"), value: fmt(d.attitude.gs, 1), unit: "kts" },
        ]} />
        <SectionTitle>{t("debug.sensors")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.onGround"), value: <BoolBadge value={d.sensors.onGround} /> },
          { label: t("debug.row.stall"), value: <BoolBadge value={d.sensors.stallWarning} /> },
          { label: t("debug.row.overspeed"), value: <BoolBadge value={d.sensors.overspeedWarning} /> },
        ]} />
      </Group>

      <Group title={t("debug.group.systems")}>
        <SectionTitle>{t("debug.engines")}</SectionTitle>
        <div className="rounded-md border border-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/50">
                <th className="px-2 py-1 text-left text-[10px] font-medium text-muted-foreground">{t("debug.row.engine")}</th>
                <th className={th}>{t("debug.row.running")}</th>
                <th className={th}>{t("debug.row.n1")}</th>
                <th className={th}>{t("debug.row.n2")}</th>
                <th className={th}>{t("debug.row.throttle")}</th>
                <th className={th}>{t("debug.row.mixture")}</th>
                <th className={th}>{t("debug.row.prop")}</th>
              </tr>
            </thead>
            <tbody>
              {d.engines.map((eng, i) => (
                <tr key={i} className="border-b border-border/50 last:border-0">
                  <td className="px-2 py-1 font-mono text-xs">{i + 1}</td>
                  <td className="px-2 py-1 text-right"><BoolBadge value={eng.running} /></td>
                  <td className={td}>{fmt(eng.n1, 1)}</td>
                  <td className={td}>{fmt(eng.n2, 1)}</td>
                  <td className={td}>{fmt(eng.throttlePos, 0)}</td>
                  <td className={td}>{fmt(eng.mixturePos, 0)}</td>
                  <td className={td}>{fmt(eng.propPos, 0)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <SectionTitle>{t("debug.apu")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.apuSwitch"), value: <BoolBadge value={d.apu.switchOn} /> },
          { label: t("debug.row.apuRpm"), value: fmt(d.apu.rpmPercent, 1), unit: "%" },
          { label: t("debug.row.genSwitch"), value: <BoolBadge value={d.apu.genSwitch} /> },
          { label: t("debug.row.genActive"), value: <BoolBadge value={d.apu.genActive} /> },
        ]} />
        <SectionTitle>{t("debug.doors")}</SectionTitle>
        <DataTable rows={d.doors.map((door, i) => ({
          label: t("debug.row.door", { n: i + 1 }),
          value: fmt(door.openRatio * 100, 0),
          unit: "%",
        }))} />
        <SectionTitle>{t("debug.weight")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.total"), value: fmt(d.weight?.totalWeight ?? 0, 0), unit: "lbs" },
          { label: t("debug.row.fuel"), value: fmt(d.weight?.fuelWeight ?? 0, 0), unit: "lbs" },
        ]} />
      </Group>

      <Group title={t("debug.group.radios")}>
        <SectionTitle>{t("debug.radios")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.com1"), value: fmt(d.radios.com1, 3), unit: "MHz" },
          { label: t("debug.row.com2"), value: fmt(d.radios.com2, 3), unit: "MHz" },
          { label: t("debug.row.nav1"), value: fmt(d.radios.nav1, 2), unit: "MHz" },
          { label: t("debug.row.nav2"), value: fmt(d.radios.nav2, 2), unit: "MHz" },
          { label: t("debug.row.nav1Obs"), value: fmt(d.radios.nav1OBS, 0), unit: "deg" },
          { label: t("debug.row.nav2Obs"), value: fmt(d.radios.nav2OBS, 0), unit: "deg" },
          { label: t("debug.row.xpdrCode"), value: fmt(d.radios.xpdrCode, 0) },
          { label: t("debug.row.xpdrState"), value: d.radios.xpdrState || "—" },
        ]} />
        <SectionTitle>{t("debug.autopilot")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.apMaster"), value: <BoolBadge value={d.autopilot.master} /> },
          { label: t("debug.row.heading"), value: fmt(d.autopilot.heading, 0), unit: "deg" },
          { label: t("debug.row.altitude"), value: fmt(d.autopilot.altitude, 0), unit: "ft" },
          { label: t("debug.row.vs"), value: fmt(d.autopilot.vs, 0), unit: "fpm" },
          { label: t("debug.row.speed"), value: fmt(d.autopilot.speed, 0), unit: "kts" },
          { label: t("debug.row.approach"), value: <BoolBadge value={d.autopilot.approachHold} /> },
          { label: t("debug.row.navLock"), value: <BoolBadge value={d.autopilot.navLock} /> },
        ]} />
      </Group>

      <Group title={t("debug.group.lightsControls")}>
        <SectionTitle>{t("debug.lights")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.beacon"), value: <BoolBadge value={d.lights.beacon} /> },
          { label: t("debug.row.strobe"), value: <BoolBadge value={d.lights.strobe} /> },
          { label: t("debug.row.landing"), value: <BoolBadge value={d.lights.landing} /> },
        ]} />
        <SectionTitle>{t("debug.controls")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.elevator"), value: fmt(d.controls.elevator, 3) },
          { label: t("debug.row.aileron"), value: fmt(d.controls.aileron, 3) },
          { label: t("debug.row.rudder"), value: fmt(d.controls.rudder, 3) },
          { label: t("debug.row.flaps"), value: fmt(d.controls.flaps, 0), unit: "%" },
          { label: t("debug.row.spoilers"), value: fmt(d.controls.spoilers, 0), unit: "%" },
          { label: t("debug.row.gearDown"), value: <BoolBadge value={d.controls.gearDown} /> },
        ]} />
        <SectionTitle>{t("debug.misc")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.aircraft"), value: d.aircraftName || "—" },
          { label: t("debug.row.altimeter"), value: fmt(d.altimeterInHg, 2), unit: "inHg" },
          { label: t("debug.row.zulu"), value: fmt(d.simTime.zuluTime, 0), unit: "sec" },
          { label: t("debug.row.local"), value: fmt(d.simTime.localTime, 0), unit: "sec" },
        ]} />
      </Group>
    </div>
  );
}
```

(Doors were labelled from 0; `Door {{n}}` counts from 1 like the engines.)

- [ ] **Step 10: The container** — replace all of `frontend/src/components/debug-tab.tsx` with:

```tsx
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Events } from "@wailsio/runtime";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useFlightData } from "@/hooks/use-flight-data";
import { useDebugSnapshot } from "@/hooks/use-debug-snapshot";
import { FlightDataService } from "../../bindings/airspace-acars";
import { DebugStatusStrip } from "./debug/debug-status-strip";
import { DebugOverview } from "./debug/debug-overview";
import { DebugTelemetry } from "./debug/debug-telemetry";
import { DebugPayload } from "./debug/debug-payload";
import { DebugLogs } from "./debug/debug-logs";

export function DebugTab() {
  const { t } = useTranslation();
  const { flightData } = useFlightData();
  const snapshot = useDebugSnapshot();
  const [connected, setConnected] = useState(false);
  const [adapter, setAdapter] = useState("");

  useEffect(() => {
    let cancelled = false;
    Promise.resolve(FlightDataService.IsConnected())
      .then((c) => { if (!cancelled) setConnected(c); })
      .catch(() => {});
    Promise.resolve(FlightDataService.ConnectedAdapter())
      .then((name) => { if (!cancelled) setAdapter(name); })
      .catch(() => {});
    // "connection-state" carries the adapter name, or "" when the link is lost.
    const off = Events.On("connection-state", (event: { data: unknown }) => {
      const name = typeof event.data === "string" ? event.data : "";
      setConnected(name !== "");
      setAdapter(name);
    });
    return () => { cancelled = true; off(); };
  }, []);

  // Re-rendered by every snapshot poll, so ages stay current.
  const now = Date.now();

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">{t("debug.title")}</h2>
        <p className="text-sm text-muted-foreground">{t("debug.subtitle")}</p>
      </div>
      <DebugStatusStrip connected={connected} adapter={adapter} flightData={flightData} snapshot={snapshot} now={now} />
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">{t("debug.tab.overview")}</TabsTrigger>
          <TabsTrigger value="telemetry">{t("debug.tab.telemetry")}</TabsTrigger>
          <TabsTrigger value="payload">{t("debug.tab.payload")}</TabsTrigger>
          <TabsTrigger value="logs">{t("debug.tab.logs")}</TabsTrigger>
        </TabsList>
        <TabsContent value="overview" className="pt-3">
          <DebugOverview snapshot={snapshot} flightData={flightData} now={now} />
        </TabsContent>
        <TabsContent value="telemetry" className="pt-3">
          {flightData
            ? <DebugTelemetry flightData={flightData} />
            : <p className="text-sm text-muted-foreground">{t("debug.waitingForData")}</p>}
        </TabsContent>
        <TabsContent value="payload" className="pt-3">
          <DebugPayload report={snapshot?.report} now={now} />
        </TabsContent>
        <TabsContent value="logs" className="pt-3">
          <DebugLogs />
        </TabsContent>
      </Tabs>
    </div>
  );
}
```

If `useFlightData` does not export the `FlightData` type, add `export` to its `interface FlightData` declaration in `src/hooks/use-flight-data.ts`.

- [ ] **Step 11: Run the frontend tests**

Run: `cd frontend && npm test`
Expected: all suites PASS, including `debug-tab.test.tsx` (5 tests) and the new parity test.

- [ ] **Step 12: Commit**

```bash
git add frontend/src/components/debug-tab.tsx frontend/src/components/debug frontend/src/components/debug-tab.test.tsx frontend/src/locales frontend/src/hooks/use-flight-data.ts
git commit -m "feat(frontend): reorganise the debug tab around ground position and the sent report"
```

---

### Task 5: Docs, full verification, live check

**Files:**
- Modify: `docs/runway-stand.md`, `README.md`
- Add: `docs/plans/2026-10-08-debug-screen-design.md`, `docs/plans/2026-10-08-debug-screen-plan.md`

- [ ] **Step 1: Docs** — in `docs/runway-stand.md`, "Where the data comes from", replace the first paragraph with:

```markdown
The airport is looked up whenever a simulator is connected, flight or not,
while the aircraft is on the ground or below 2,500 ft AGL, so the runway is
already known at touchdown. It is looked up again once the aircraft leaves
the airport's area. Lookups are at least 30 s apart, so the first second or
two after arriving somewhere new show no runway or stand.

The **Debug** tab shows the result: the airport, runway and stand under
Overview, and under Payload the last position report exactly as it was sent
(or, before a flight, the one that would be sent), with whether it was sent,
queued in the outbox or dropped.
```

In `README.md`'s feature list add after the Aircraft profiles line:

```markdown
- **Debug tab** — Live ground position (airport, runway, stand), the last position report exactly as sent with its outcome, telemetry and logs
```

- [ ] **Step 2: Full verification (AGENTS §8.2)**

```bash
PKGS=$(go list ./... | grep -v -e '/adapters/wails$' -e '^airspace-acars$' -e '/build/')
go vet $PKGS && go test $PKGS
GOOS=linux go vet ./internal/app/ ./internal/domain/
go build .
cd frontend && npm test && npx tsc --noEmit
```

Expected: Go ok; npm test all pass; `tsc` shows only the two pre-existing `auth-context.test.tsx` TS2554 errors.

- [ ] **Step 3: Live check** — with MSFS running and the aircraft at a gate, run `"$(go env GOPATH)/bin/wails3" dev -config ./build/config.yml -port 9245`, open Debug: the strip and Overview show the airport and the stand; Payload shows the preview JSON with `"stand": {"name": ...}`. Then restore any `go.mod` / `frontend/package-lock.json` changes the dev run makes (`git restore go.mod frontend/package-lock.json`).

- [ ] **Step 4: Commit**

```bash
git add docs/runway-stand.md README.md docs/plans/2026-10-08-debug-screen-design.md docs/plans/2026-10-08-debug-screen-plan.md
git commit -m "docs(position): describe the debug tab's ground position and report"
```
