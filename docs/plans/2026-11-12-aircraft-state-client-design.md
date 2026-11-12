# Aircraft State Client: the ACARS keeps the aircraft as the airline says it is

Status: proposed. Airspace side: FerrLab/airspace,
`docs/plans/2026-11-12-aircraft-state-client-design.md`, branch
`claude/compassionate-darwin-sxliyf`. That note owns the API contract; the copy
below is for reading convenience.

## Goal

Airspace already tracks each aircraft's parts, the defects filed against them
and the MEL items they are deferred under. None of that reaches the simulator:
a pilot can book an A320 with PACK 1 deferred and fly it with both packs on.

The Aircraft State Client closes that gap. When a flight starts, the ACARS
fetches the booked aircraft's state from Airspace, works out what that means
for the add-on aircraft loaded in the simulator, writes it into the sim and
keeps watching it for the rest of the flight. The pilot sees, in a new tab,
what is being applied, where in the cockpit, and why.

The first kind of state is **inoperative parts** (INOP items): a part with an
active defect is held in the position its MEL procedure prescribes, e.g.
"PACK 1 pushbutton OFF". The design leaves room for the second kind, **wear
levels** (brake and tyre wear, oil quantity and the like, set at the start of
a flight and read back at the end), sketched under [Phase 3](#phase-3-wear-levels).

## Out of scope

- **Failures.** We do not trigger failures, ECAM cautions or anything that
  needs an add-on's own failure system. An INOP item is a placarded control,
  nothing more.
- Defects filed *during* the flight. The state is taken when the flight
  starts; a defect filed later is shown, never written. Writing it mid-flight
  would be a failure.
- MEL items with no part (an `AircraftMelItem` raised by hand without a
  defect). They have nothing to map to.
- K: events, RPN calculator code and anything that needs the MobiFlight WASM
  module or a WASM module of our own. Phase 1 writes variables only.
- Integrating with TechLog or any other maintenance add-on. TechLog is
  closed-source and has no published interface (checked 2026-11-12).
- Releasing held controls when the flight ends. The ACARS never "restores" a
  switch: turning a pack back on behind the pilot's back is worse than
  leaving it off.

## Concepts

| Term | Meaning | Owned by |
|---|---|---|
| **Model** | A family of add-on aircraft that share one cockpit: `fenix-a32x`, `fslabs-a32x`, `toliss-a32x`. | ACARS catalogue |
| **Item** | One thing on a model the client can hold or watch, with a stable key and a number of positions: `air.pack` (2), `nav.wxr` (1). | ACARS catalogue |
| **Recipe** | How an item's position is held on a model: which variable to write, to what value, and what to read to confirm it. | ACARS catalogue, never sent by the server |
| **Mapping** | "Our part PACK is `air.pack` on `fenix-a32x` and `air.pack` on `fslabs-a32x`." Made by the airline's staff on the part. | Airspace, per tenant |
| **State** | The booked aircraft's active defects, each with its part, position and mappings. | Airspace, per booking |

A part's positions map one to one onto the item's positions: defect on PACK
position 2 → `air.pack` position 2. Airspace refuses a mapping whose item has
fewer positions than the part has installed.

### Why the recipes stay in the ACARS

The first idea was for the backend to store the variable and value on the
part. The recipes live in the ACARS instead:

1. **No remote writes.** Any staff member of any tenant, or anyone who
   compromises the server, could otherwise make every pilot's ACARS write
   arbitrary variables into their simulator. With the recipes in the
   catalogue, the server can only pick from writes we shipped and tested.
2. **One part, many aircraft.** A part is shared across subfleets and an
   airline flies them in different add-ons. The same PACK part is one LVar on
   the Fenix, another on the FSLabs and a dataref on the ToLiss.
3. **One fix for everyone.** When an add-on update renames a variable, one
   ACARS release fixes it for every airline, instead of every airline's staff
   editing their parts.

The cost is that a new item needs an ACARS release and a catalogue update on
Airspace. Items change far less often than defects, so this is acceptable.

## The catalogue

One JSON file per model under `internal/aircraftstate/catalog/`, embedded in
the binary. Variable names below are placeholders: the real ones come from
HubHop's input presets and are confirmed in the simulator before an item
ships.

```json
{
  "model": "fenix-a32x",
  "name": "Fenix A318/A319/A320/A321",
  "sim": "simconnect",
  "profiles": ["fenix-a32x"],
  "items": [
    {
      "key": "air.pack",
      "name": "Air conditioning pack",
      "ata": "21",
      "positions": 2,
      "inop": {
        "mode": "hold",
        "positions": {
          "1": {
            "control": "PACK 1 pushbutton",
            "set":     { "kind": "lvar", "name": "<PACK 1 pushbutton>", "value": 0 },
            "confirm": { "kind": "lvar", "name": "<PACK 1 OFF light>", "equals": 1 }
          },
          "2": { "control": "PACK 2 pushbutton", "set": { … }, "confirm": { … } }
        }
      }
    },
    {
      "key": "brakes.autobrake",
      "name": "Autobrake",
      "ata": "32",
      "positions": 1,
      "inop": {
        "mode": "watch",
        "positions": {
          "1": { "control": "AUTO/BRK pushbuttons",
                 "watch": { "kind": "lvar", "name": "<autobrake mode>", "used_when": { "op": "ne", "value": 0 } } }
        }
      }
    },
    {
      "key": "cabin.lav",
      "name": "Lavatory",
      "ata": "38",
      "positions": 4,
      "inop": { "mode": "placard" }
    }
  ]
}
```

- **`profiles`**: the model applies when the aircraft profile in force
  (`profiles.Plan.ProfileIDs()`) contains one of these IDs. The aircraft is
  identified once, by the profile system, not again here.
- **Modes:**
  - **`hold`** is for latching controls. The client writes `set` and puts it
    back when it moves.
  - **`watch`** is for momentary buttons and selectors, where writing back
    would make the control flicker. The client only records each use.
  - **`placard`** is for items with no cockpit control. The client shows them
    and enforces nothing.
- **`confirm`** reads something *other* than what was written: the annunciator
  light or the system state. SimConnect creates a local variable that does
  not exist instead of rejecting it, so writing a misspelt name "succeeds"
  and holds nothing. The confirmation is how the client tells a working
  recipe from a broken one.
- **Write kinds**, phase 1: `lvar` and settable `simvar` on MSFS, `dataref` on
  X-Plane.
- **`control`** is the label on the panel. It is shown to the pilot as data,
  like part names from Airspace, and is not translated: cockpit labels are
  English in the aircraft too.
- **Keys are permanent.** Airspace stores them, so a key is never renamed or
  reused. A retired item keeps its entry with `"retired": true`: it can no
  longer be chosen, and existing mappings still resolve.

`go test ./internal/aircraftstate/...` validates every file:
- every listed profile ID exists in `internal/profiles/builtin/`;
- every position from 1 to `positions` has a recipe;
- write kinds suit the model's simulator;
- keys are unique;
- the export described next is up to date.

### Export for Airspace

`go run ./cmd/aircraftstate export > catalog.json` writes the catalogue
without recipes: per model its key, name and simulator, and per item its key,
name, ATA chapter, positions, INOP mode, control labels and `retired`.
Airspace vendors that file (see its note). A golden test fails when the
committed export is stale, so a catalogue change and its export land in the
same PR. Copying it to Airspace is a PR there; automating that is a
follow-up.

## API (Airspace, pilot token, `/api/v2/acars`)

Canonical in the Airspace note. Both routes answer
`404 {"message": "No active booking"}` without a booking.

### `GET /aircraft-state`

```json
{
  "data": {
    "version": 1,
    "revision": "4be1…",
    "booking_id": 123,
    "aircraft": { "id": 55, "registration": "CS-TNV", "subfleet": "A320-200" },
    "inop": [
      {
        "defect_id": 901,
        "status": "deferred",
        "part": { "id": 12, "code": "PACK", "name": "Air conditioning pack", "ata_chapter": "21" },
        "position": 1,
        "equipment": "PACK 1",
        "reason": "Pack outlet temperature high",
        "reported_at": "2026-11-10T14:02:00Z",
        "mel": {
          "ata_code": "21-51-01",
          "title": "Air conditioning pack",
          "repair_category": "C",
          "due_at": "2026-11-20T14:02:00Z",
          "operational_procedure": "Pack selected OFF…",
          "operational_procedure_document_id": 31
        },
        "sim": [
          { "model": "fenix-a32x",  "item": "air.pack" },
          { "model": "fslabs-a32x", "item": "air.pack" }
        ]
      }
    ]
  }
}
```

- **`inop`** lists every active defect on the aircraft: `open`, `deferred` or
  `nogo`; rectified ones are left out. A defect with no part has `part: null`.
  A part without a mapping has `sim: []`. Both are still shown, as placards.
- **`position`** is `null` for a part with a single installed position.
- **`revision`** changes whenever anything in `data` changes, so the client
  can tell a refresh that changes nothing from one that does.

### `POST /aircraft-state/report`

What the client did, sent while the flight runs and at the end (see
[Reporting](#reporting)). Idempotent: each report replaces the previous one
for the booking.

## Components

Dependency direction stays domain ← app ← adapters ← main.

### `internal/aircraftstate` (new package)

Pure and free of goroutines, so every rule is tested with a fake clock and
no simulator. It imports `domain` and nothing else from `internal/`.

| File | Contents |
|---|---|
| `catalog.go` | Schema, `//go:embed catalog/*.json`, `Load`, `Validate`. |
| `export.go` | The recipe-free export Airspace vendors. |
| `resolve.go` | `Resolve(state domain.AircraftState, profileIDs []string, sim string) Plan`. For each INOP entry it finds the model in force and the item's recipe for that position, or the reason there is none: `no_model` (the loaded aircraft has no catalogue), `not_mapped` (no mapping for this model), `unknown_item` (a mapping to a key this build does not know, e.g. an older ACARS), `placard`, `no_part`. |
| `enforcer.go` | The `Enforcer` state machine (below). `Observe(now, readings) []domain.SimWrite` returns the writes due this tick and updates each target's status. |
| `report.go` | The report sent to Airspace, built from the enforcer's targets. |

#### Enforcer, per held target

```
waiting ──settle delay──▶ applying ──confirm true──▶ held
                              │                      │  reading ≠ set
                              │ no confirm after      ▼
                              │ confirmTimeout      moved ──write back──▶ held
                              ▼                      │  > maxReverts in revertWindow
                         unconfirmed                 ▼
                                                  overridden (watch only, no writes)
```

- **Settle first.** The first write waits `settleDelay` after the profile is
  applied. Fenix sets every switch when it loads a panel state (cold and
  dark, turnaround), and writing earlier races it.
- **Write on difference.** A write happens only when the reading differs from
  `set`, at most once per tick per target.
- **Unconfirmed.** After `confirmTimeout` without the confirmation, the target
  is `unconfirmed`: the recipe does not work on this build of the add-on.
  The client keeps writing (it may be harmless) but tells the pilot that the
  item is *not* enforced, so nobody believes it is.
- **Overridden.** After `maxReverts` within `revertWindow`, the target turns
  `overridden`. The client stops writing and only records, because something
  else is holding the same control: the pilot on purpose, a MobiFlight panel,
  or another add-on such as TechLog. Two programs flipping a switch back and
  forth for an hour is worse than letting go.
- **Watch targets** go straight to `watching` and count each time `used_when`
  becomes true.

The timing constants are package-level with doc comments, and tests shorten
them through a helper. Starting values:

| Constant | Value | Why |
|---|---|---|
| `settleDelay` | 20 s | Longer than a Fenix panel-state load, measured on a turnaround state. |
| `confirmTimeout` | 10 s | Fenix lights follow a pushbutton within a second or two. |
| `maxReverts` / `revertWindow` | 5 / 2 min | Several quick reverts means a fight, not a curious pilot. |

### Simulator adapters

The write capability is a consumer-owned interface in `internal/app`,
asserted on the adapter the same way `profileAware` is, so an adapter
without it simply holds nothing:

```go
// stateAware is implemented by simulator adapters that can write to the
// aircraft and read the variables that confirm it.
type stateAware interface {
	// WatchState replaces the set of variables read for the aircraft state.
	WatchState(vars []domain.SimVarRef) error
	// StateReadings returns the latest reading of each watched variable by key.
	StateReadings() map[string]float64
	// WriteState queues one write; it runs on the adapter's own thread.
	WriteState(w domain.SimWrite) error
}
```

- **`internal/simconnect`** gains `SimConnect_SetDataOnSimObject`, loaded
  alongside the existing procs.
- **`internal/adapters/simconnect`**
  - Watched variables get a data definition of their own, separate from the
    profile's, so a profile re-plan does not drop them and they do not drop
    the profile's.
  - Each write uses a one-variable definition (`L:` prefix for local
    variables, as `datumName` already does) and `SetDataOnSimObject` on the
    user aircraft.
  - Writes are queued and run on the dispatch thread, like
    `installPendingPlan`.
  - Windows-only behind the existing build tags; `adapter_stub.go` gets the
    no-op methods.
- **`internal/adapters/xplane`**
  - Writes are a `DREF` packet.
  - Readbacks join the existing `RREF` subscription.

`domain` gains `SimVarRef`, `SimWrite`, `AircraftState` (the decoded server
reply) and `AircraftStateSnapshot` (what the UI is sent).

### `internal/app/aircraft_state.go`

`aircraftStateClient` is modelled on `positionUploader`: constructed by `App`,
with its own mutex and its own goroutine. `App` gains one field and no new
lock.

- **Fetch.**
  - When the dashboard shows the booking (a preview, nothing written), and
    again in `StartFlight`.
  - The reply is decoded in `aircraft_state_api.go`, with a test per accepted
    variant and one for garbage.
  - A 404 means no state, not an error.
  - A transient failure is retried with backoff and never blocks the flight
    start.
- **Freeze.**
  - The reply fetched at flight start is the flight's state.
  - Later refreshes update what the tab shows but never what is written.
- **Run.**
  - A ticker of `stateTickEvery` (1 s; the switches change at human speed)
    resolves the plan against the active profile.
  - It calls `WatchState` when the plan changes, feeds `StateReadings()` to
    the enforcer and passes the writes to `WriteState`.
  - A change of aircraft profile (the pilot loads another aircraft) builds a
    new plan from the same frozen state.
- **Stop.** `StopFlight`, `FinishFlight` and a simulator disconnect stop the
  goroutine through its stop channel. Nothing is written on the way out.
- **Gates.** The client writes only while all of these hold:
  - a flight is started for this booking;
  - the simulator is connected;
  - the active profile resolves to a catalogue model;
  - the aircraft profile setting is not "off".

  The registration in the simulator is **not** a gate: pilots fly whatever
  livery they have. A mismatch is shown, not enforced.
- **UI.** It emits `aircraft-state` with `*domain.AircraftStateSnapshot` on
  every status change, at most once per second. The event is registered in
  `main.go`'s `init()`.

`services.go` gains `GetAircraftState() domain.AircraftStateSnapshot` and
`RefreshAircraftState() error`. Both return typed values, so the generated
bindings carry real types. The mock in `frontend/src/__mocks__/` is updated
to match.

### Reporting

The report says, per INOP entry, what happened:
- the final status;
- the time it was first held;
- the revert count and the time of each revert (capped at 20);
- the use count for watched items;
- the reason when not applicable.

During the flight it goes to `POST /aircraft-state/report` whenever a status
changes, debounced to once a minute. Each report replaces the last. The
latest one is kept in SQLite (`aircraft_state_report`, one row per booking)
and retried with backoff until Airspace acknowledges it.

The final report travels inside the `POST /finish` body as `aircraft_state`.
The finish is the one request the ACARS already guarantees, and Airspace
deletes the booking when it files the flight, so a separate report sent after
the finish would have nothing left to attach to.

## Observability

- **Spans:**
  - `aircraftstate.fetch` (`http.status`);
  - `aircraftstate.plan` (`state.model`, `state.items`);
  - `aircraftstate.report`.
- **Counters:**
  - `aircraftstate.write`;
  - `aircraftstate.revert`;
  - `aircraftstate.unconfirmed`;
  - `aircraftstate.overridden`.

  The same names go into `docs/observability.md`.
- **Log levels:**
  - per-tick decisions at `Debug`;
  - plan changes at `Info`;
  - `unconfirmed` once per `(model, item)` per session at `Warn`.
- **Error.** An unconfirmed recipe is our bug, so it is also reported once
  per `(model, item, catalogue version)` per session at `Error`, so Sentry
  groups it by recipe.
- **What never goes into a log, span or error:** defect `reason` text,
  operational procedures, the registration. Model and item keys are enough
  to debug a recipe.

## Frontend: the Aircraft tab

The pilot must be able to answer, at a glance: what is inoperative on this
aircraft, what is the ACARS doing about it in my cockpit, and why.

- **`components/aircraft-state-tab.tsx`** is a small container. It loads
  `GetAircraftState`, subscribes to `aircraft-state` (returning the
  unsubscribe) and remounts on tenant change with a `key`.
- **`components/aircraft-state/`**
  - `aircraft-header.tsx`: the booked registration and subfleet, the add-on
    detected and its model ("Fenix A320 — fully supported", "Aircraft not
    supported: items are shown for reference only"), and a livery mismatch
    warning.
  - `inop-list.tsx` and `inop-row.tsx`: one row per entry, showing:
    - part name and position (`PACK 1`);
    - the MEL reference, category and due date;
    - the reason the defect was filed;
    - the cockpit control and what is held ("PACK 1 pushbutton — held OFF");
    - the status badge;
    - the operational procedure, expandable.
- **`lib/aircraft-state.ts`** (with `.test.ts`) holds the pure logic:
  - status → badge and translation key;
  - sorting (held and moved first, then placards);
  - the "N INOP" summary for the sidebar and dashboard.

Statuses shown: *Waiting for the simulator*, *Applying*, *Held*, *Moved —
put back (n)*, *Not confirmed — not enforced*, *Released — held by something
else*, *Watching — used n times*, *Placard only*, *No mapping for this
aircraft*, *Aircraft not supported*. Every string is under `aircraftState.*`
in `en`, `es`, `pt` and `fr`, with the prefix added to
`locale-parity.test.ts`.

The sidebar gets an *Aircraft* entry with the INOP count as a badge, and the
dashboard a one-line summary that opens the tab.

## Phases

1. **INOP, Fenix.**
   - Catalogue with a handful of verified Fenix A32x items: packs, APU bleed,
     a fuel pump, weather radar, a generator.
   - The package, the SimConnect write path, the client and the tab.
   - Airspace: mapping, catalogue, `GET /aircraft-state`.
   - Behind a beta switch in Settings until the Windows path is proven.
2. **Reporting.** `POST /aircraft-state/report`, the SQLite retry and the
   report on the flight review in Airspace.
3. **Wear levels** (below).
4. **More models:**
   - ToLiss A32x on X-Plane: datarefs, no new transport.
   - FSLabs, once we know whether its cockpit is driven by variables.
   - A HubHop step in `cmd/hubhop` to draft recipes from input presets.

### Phase 3: wear levels

The same endpoint gains a `levels` list: per aircraft, values the airline
tracks between flights, e.g. `{"key": "brakes.wear", "value": 0.42}`.

Catalogue items gain a `level` section: what to read, what to write, unit
and range.

The client:
- writes each level once, after the settle delay;
- reads it back at finish;
- reports the values in the same report.

Airspace keeps the reported values as the aircraft's state for its next
flight. Nothing is held during the flight, so it reuses the plan, the
adapters and the reporting unchanged.

Which values each add-on lets us write is the open question. That research
comes before this phase is planned in detail.

## Verification

- **Go**, with the commands in AGENTS.md §8.2. New tests:
  - catalogue validation;
  - resolve, one per reason;
  - enforcer, with a fake clock for every transition;
  - the reply parser, one per variant plus garbage;
  - the client against fake `stateAware` and `AirspaceAPI` stubs;
  - report retry.
- **Frontend:** `npm test`, with the tab queried by role and visible text.
- **Windows (manual).** Before phase 1 ships, on the Fenix:
  - every item holds;
  - the confirmation lights;
  - a turnaround state load does not beat the settle delay;
  - a fight with a MobiFlight panel ends in `overridden`.

  Recorded in the PR.

## Open questions

- Does `SetDataOnSimObject` on an `L:` definition drive the Fenix switches
  the way MobiFlight's writes do? This is the first thing to spike on Windows;
  the fallback is the MobiFlight WASM module.
- Should the airline be able to make the state mandatory (no beta switch, no
  "off" profile)? Phase 1 leaves it to the pilot and reports when nothing was
  applied.
