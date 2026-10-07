# Runway and stand in position reports

## Goal

Each position report gets two new top-level keys:

- `runway`: `null` unless the aircraft is on a runway. When it is, the
  designator and physical-end position (lat/lng/alt) of both ends.
- `stand`: `null` unless the aircraft is on a parking stand. When it is, the
  stand name and its lat/lng.

The data comes from the simulator's own scenery. On MSFS that means the
SimConnect Facilities API. On X-Plane, UDP cannot carry airport layout, so it
means the sim's `apt.dat` files.

## Wire format

Units follow the existing `{value, unit}` measurement shape:

```json
"runway": {
  "ends": [
    { "designator": "09L",
      "latitude":  {"value": 51.4775, "unit": "deg"},
      "longitude": {"value": -0.4851, "unit": "deg"},
      "altitude":  {"value": 79,      "unit": "ft"} },
    { "designator": "27R", "latitude": ..., "longitude": ..., "altitude": ... }
  ]
},
"stand": {
  "name": "A12",
  "latitude":  {"value": 51.4712, "unit": "deg"},
  "longitude": {"value": -0.4598, "unit": "deg"}
}
```

Reports that are already in the outbox carry neither key. The server MUST
treat both as optional, so that "absent" means the same as `null`.

**Altitude caveat.** Neither source gives a separate elevation for each runway
end:
- MSFS `RUNWAY.ALTITUDE` is the elevation at the runway centre.
- `apt.dat` has only the airport elevation (row 1).

Both ends therefore report the same value. The field is still sent, so the
shape stays the same if a better source turns up later.

## Approach

### Domain (`internal/domain/airport_layout.go`)

Plain types and pure functions. There is no I/O here, so all of it is
table-tested.

- `AirportLayout{ICAO, RefLat, RefLon, ElevationFt, Runways []Runway, Stands []Stand}`
- `Runway{Ends [2]RunwayEnd, WidthM float64}`, where
  `RunwayEnd{Designator string, Lat, Lon, AltFt float64}`. Both sources are
  normalised to the two physical runway ends:
  - MSFS gives a centre, a heading and a length. We compute the ends with a
    great-circle destination.
  - `apt.dat` gives the ends directly.
- `Stand{Name string, Lat, Lon, RadiusM float64}`
- `LocateRunway(layout, lat, lon) *Runway`: point-in-rectangle along the
  runway's centreline, with a half-width tolerance.
- `LocateStand(layout, lat, lon) *Stand`: the nearest stand whose radius
  contains the point.
- `AirportLayoutProvider` interface (consumer-owned, §3.1):
  `NearestAirportLayout(ctx, lat, lon) (*AirportLayout, error)`.
  - It is **optional**. The app type-asserts the connector against it, so
    `SimConnector` and every test stub stay unchanged.

### App (`internal/app/ground_locator.go`)

`groundLocator` is a new type with its own mutex (§3.2). It holds the cached
layout of the current airport.

- `Observe(fd *FlightData) (runway *Runway, stand *Stand)` is called from
  `buildPositionReport`.
  - If the aircraft is not on the ground, it returns nil/nil and does nothing
    else.
  - If the cached layout is missing, more than 5 NM away, or older than 10 min,
    it asks its goroutine to fetch a fresh one. It does not block: until the
    fetch finishes, the report sends `null`.
  - It then calls the two domain functions against the cached layout.
- The fetch goroutine starts with `defer observability.Recover()`. It stops on
  `stopCh`, and each fetch is wrapped in an `airport.layout.fetch` span.
- Failures are classified:
  - A missing `apt.dat` or no airport nearby is `span.Expected`, logged at
    `Debug` after the first `Warn`.
  - A parse error is `span.Fail`.
- `buildPositionReport` gets two lines that add the keys. The
  `map[string]interface{}` there is legacy; the new sub-objects are small
  typed structs with JSON tags.

### MSFS (`internal/simconnect`, `internal/adapters/simconnect`)

- Bind `SimConnect_RequestFacilitiesList_EX1`, `SimConnect_AddToFacilityDefinition`
  and `SimConnect_RequestFacilityData`. Add the recv IDs for `AIRPORT_LIST`,
  `FACILITY_DATA` and `FACILITY_DATA_END`, and their structs.
  - The enum values must be checked against `SimConnect.h` from the SDK. They
    will not come from memory.
- The facility definition is:
  `OPEN AIRPORT` / `LATITUDE` / `LONGITUDE` / `ALTITUDE`
  / `OPEN RUNWAY` (lat, lon, alt, heading, length, width,
  primary/secondary number and designator) / `CLOSE RUNWAY`
  / `OPEN TAXI_PARKING` (name, number, suffix, bias_x, bias_z, radius)
  / `CLOSE TAXI_PARKING` / `CLOSE AIRPORT`.
- SimConnect is single-threaded per handle. The adapter's `run()` loop
  therefore owns every call. `NearestAirportLayout` sends a request on a
  channel and waits for the reply, with a context timeout of 5 s. The dispatch
  switch assembles `FACILITY_DATA` rows until `FACILITY_DATA_END`.
- The nearest airport is the closest entry in the reality-bubble airport list.
- The stand name is built as the `NAME` enum's prefix, then `NUMBER`, then
  `SUFFIX`. For example, `GATE_A` with 12 gives `A12`, and `PARKING` with 5
  gives `5`. The prefix table lives in domain, with tests.
- Stand position is the airport reference point plus `BIAS_X`/`BIAS_Z`
  metres, applied as east/north offsets.

### X-Plane (`internal/adapters/xplane/aptdat`)

This is a new sub-package. It imports only domain.

- **Locating the files.** X-Plane writes `%LOCALAPPDATA%\x-plane_install_12.txt`
  (falling back to `_11`), and that file lists the install roots. A
  `xplanePath` setting overrides the auto-detected path.
- **Scenery order.**
  - First, the `Custom Scenery/*/Earth nav data/apt.dat` packs, in
    `Custom Scenery/scenery_packs.ini` order.
  - Then `Global Scenery/Global Airports/Earth nav data/apt.dat`.
  - The first pack that defines an ICAO wins, which matches the sim.
- **Index.** One streaming pass, run lazily on the first lookup and in the
  background, builds `ICAO → (file, byte offset, ref lat/lon)`.
  - The reference point comes from row 1302 `datum_lat`/`datum_lon`. If that
    is missing, it is the mean of the runway ends.
  - The XP12 global file is about 300 MB, so the index keeps offsets, not
    layouts.
- **Parsing an airport.**
  - Row 1 gives the elevation.
  - Row 100 gives the width, and per end the number, lat and lon.
  - Row 1300 gives the stand lat, lon and name.
  - `apt.dat` has no stand radius, so we use `defaultStandRadiusM`, a
    documented constant.
- Reads are bounded and cleaned up with `defer`. File paths are never logged,
  because they contain the profile directory (§4.3).
- **Settings.** A new `xplanePath` field gets one input in Settings and its
  string in all four locales.

## Span points

- `airport.layout.fetch`: `sim.adapter`, `airport.icao`, and outcome.
- `aptdat.index.build`: `aptdat.files` and `aptdat.airports` (counts, no
  paths).
- `simconnect.facility.request`: `facility.kind`.

## Tests

- **Domain:** a table test for runway containment (centreline, edge, just
  outside, beyond an end, across the antimeridian), stand radius, and the MSFS
  stand-name formatting.
- **App:** `groundLocator` with a fake provider covers these cases:
  - airborne gives null;
  - a stale layout triggers a refetch;
  - a failing provider leaves the report null and does not spam the logs.
- **aptdat:** fixture files under `testdata/` cover these cases:
  - custom scenery overrides global;
  - a missing row 1302 falls back to the runway mean;
  - truncated or garbage lines are rejected while the rest of the airport
    survives.
- **SimConnect:** the facility-row assembly is a pure function, so it is
  tested without the DLL. The live path is **Windows-only and untested in CI**.

## Out of scope

- Displaced thresholds. We send the physical ends; the server can apply
  offsets if it needs them.
- Runway or stand display in the webview.
- Taxiway names, and runway occupancy while airborne (for example, over the
  threshold at 10 ft).
- Per-end runway elevation, which neither source provides.

## Changes made during implementation

These differ from the approved design above:

- **Per-end runway elevation on X-Plane.** This comes from CIFP `RWY:`
  records. A pilot's AIRAC in `Custom Data/CIFP` takes priority over
  `Resources/default data/CIFP`.
  - We chose CIFP over GNS430 `Airports.txt` because XP12 no longer ships
    GNS430 data, while every install has CIFP.
  - MSFS still reports the runway-centre elevation for both ends.
- **Finding the X-Plane folder.** The folder comes from the running
  `X-Plane.exe` process (`/proc` on Linux) instead of
  `x-plane_install_12.txt`. That file lists every install on the machine,
  not the one in use.
- **No Settings UI for `xplanePath`.** It lives in `settings.json`, next to
  `xplaneHost`/`xplanePort`, which the UI does not show either. As a result
  there are no new locale strings.
- **MSFS decoding.** Runways and parking are fetched with two separate
  facility definitions.
  - Each row is either the airport (no parent) or one kind of child.
  - The payload is found from the message size, never from header offsets.
    The header differs between MSFS 2020 and 2024, and its packing is
    undocumented.
- **Choosing the X-Plane airport.** An airport whose runways and stands
  surround the aircraft wins. Failing that, the nearest reference point
  within 10 km.

User-facing documentation: `docs/runway-stand.md`.
