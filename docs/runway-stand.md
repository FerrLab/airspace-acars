# Runway and stand in position reports

Every position report sent to `/api/v2/acars/position` has two keys that say
where on the airport the aircraft is. Each one is `null` unless the aircraft
is on the ground on a runway or a stand:

```json
"runway": {
  "ends": [
    { "designator": "09L",
      "latitude":  {"value": 51.4775, "unit": "deg"},
      "longitude": {"value": -0.4850, "unit": "deg"},
      "altitude":  {"value": 79,      "unit": "ft"} },
    { "designator": "27R", "latitude": {...}, "longitude": {...}, "altitude": {...} }
  ]
},
"stand": {
  "name": "A12",
  "latitude":  {"value": 51.4712, "unit": "deg"},
  "longitude": {"value": -0.4598, "unit": "deg"}
}
```

Reports queued in the outbox by an older version of the ACARS have neither
key, so the server must treat a missing key as `null`.

## Meaning of the fields

- **Runway ends** are the *physical* ends of the pavement, not displaced
  landing thresholds. They are listed in the order the scenery gives them;
  the first is usually the lower-numbered end.
- **Runway altitude:**
  - **MSFS:** both ends carry the elevation of the runway centre. The
    Facilities API has no elevation for each end.
  - **X-Plane:** each end gets its landing-threshold elevation from the CIFP
    navdata, as listed below. The pilot's AIRAC in `Custom Data/CIFP` takes
    priority over the shipped `Resources/default data/CIFP`. Without either,
    both ends carry the airport elevation.
- **On a runway:** the aircraft is on the ground and inside the runway's
  paved rectangle, end to end and half its width either side. Where runways
  cross, it is the runway the aircraft is lined up with.
- **On a stand:** the aircraft is on the ground, within the stand's radius,
  and moving under 3 kt. That includes a pushback and leaves out an aircraft
  taxiing past the gates. When stands overlap, the nearest one wins.
  - The stand size comes from MSFS (`RADIUS`).
  - X-Plane gives every stand a 20 m radius, because `apt.dat` records stands
    as points. MSFS stands with no size get the same 20 m.
- **Stand names on MSFS** are built from the parking enums: `GATE_A` 12 is
  `A12`, `N_PARKING` 5 is `N5`, and a plain `GATE`/`PARKING` is its number.
  On X-Plane the name is the stand's name in `apt.dat` (row 1300).

## Where the data comes from

The airport is looked up when the aircraft is on the ground, or below
2,500 ft AGL so the runway is already known at touchdown. It is looked up
again once the aircraft leaves the airport's area. Lookups are at least
30 s apart, so the first report or two after arriving somewhere new may be
`null`.

**MSFS** (`internal/adapters/simconnect/facility*.go`):
- The nearest airport within 10 km comes from
  `SimConnect_RequestFacilitiesList_EX1`.
- Its runways and parking come from `SimConnect_RequestFacilityData`.
- The decoder reads the replies as bytes rather than casting them to
  structs, so the same code works with MSFS 2020 and 2024 even though their
  headers differ.

**X-Plane** (`internal/adapters/xplane/aptdat`):
- **Finding the install:** it is the folder of the running `X-Plane.exe`
  (`/proc` on Linux), unless `xplanePath` is set in `settings.json`.
- **Index:** the first lookup builds an index of every `apt.dat`, in
  background, in `Custom Scenery/scenery_packs.ini` order with the global
  airports where `*GLOBAL_AIRPORTS*` puts them. A custom scenery airport
  therefore replaces the global one, as it does in the simulator.
- **Lookups:** each one reads just that airport's rows back from its file.

## When both keys stay `null`

- **X-Plane on another machine:** process detection only sees the local
  machine. Set `xplanePath` in `settings.json` to a shared copy of the
  X-Plane folder.
- **Diagnostics:** the log says `X-Plane airport index built` with file and
  airport counts. If it says `no X-Plane airports found`, the folder has no
  readable `apt.dat`.
- **MSFS:** a warning `simulator rejected the airport facility definition`
  means the simulator does not offer the Facilities API.
