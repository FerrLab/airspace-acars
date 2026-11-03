# Flight detail tabs: Landing, Takeoff, Telemetry, Score

## Goal

Bring back the flight detail tabs that FerrLab/airspace-acars#114 dropped,
showing only values the platform recorded for the flight. Round 1 of #114's
review blocked them because every number on them was generated client-side.
The server now serves the data (FerrLab/airspace#1059):

- `GET /api/v2/acars/flights/{id}`: landing and takeoff analysis, runway and
  wind, the stabilized-approach gate, the per-category score breakdown, FDM
  events and the points ledger. A section with nothing recorded is `null`.
- `GET /api/v2/acars/flights/{id}/track`: the recorded positions, downsampled
  to at most 5000 samples.

## Approach

**Domain** (`internal/domain/flight_detail.go`). Typed structs for both
replies. Nullable server numbers are pointers, so the generated TypeScript
says `number | null` and a missing value is never shown as `0`.

**App** (`internal/app/flight_detail.go`, its own file: `flight_log.go` is at
the size signal).
- `GetFlightDetail(id)` and `GetFlightTrack(id)` go through
  `executeCandidates`/`executeRequest`, like NOTAMs. They return the same
  `status` vocabulary, so 403/404/429 surface as what they are.
- The id is validated as a single path segment.
- Decoding accepts a `data`-wrapped or flat body and a numeric or string id.
- Garbage is an error, never an empty success.
- No new state, lock or goroutine on `App`.

**Services**. Two methods on `FlightLogService`. The bindings change, and the
mock in `frontend/src/__mocks__/` grows to match.

**Webview**.
- `FlightDetailView` becomes a small container with shadcn `Tabs`: Overview
  (the current content), Landing, Takeoff, Telemetry, Score.
- It fetches the detail once per flight with the `cancelled` guard. A tab whose
  section is `null` is not rendered.
- Telemetry fetches the track only when first opened, since it is the heavy
  reply.
- Each tab is its own component under 300 lines. Formatting and chart geometry
  live in `src/lib/flight-detail-sections.ts` and `src/lib/flight-telemetry.ts`,
  with tests.
- Every string sits under `myFlights.*`, which `locale-parity.test.ts` already
  covers, in all four locales.
- Score formulas and input labels arrive already translated in the pilot's
  locale from the server, so they are shown as sent.

## Span points

`flights.get_detail` and `flights.get_track` around the request. A non-ok
status is `Expected`, not `Fail`: the airline not having the route yet is
normal operation.

## Out of scope

- Drawing the recorded track on the Overview map. `FlightsMap` takes logbook
  entries, not tracks, so that is a change to the map component on its own.
- The staff-only Log tab and notes, which the server does not expose to pilots.
- Charting more than altitude and speed. The track carries only what the map
  and a profile chart need.
