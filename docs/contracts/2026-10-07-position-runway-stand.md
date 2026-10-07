# Contract update: `runway` and `stand` in ACARS position reports

| | |
|---|---|
| Endpoint | `POST /api/v2/acars/position` |
| Direction | ACARS → backend |
| Change | Two new top-level keys on each report: `runway` and `stand` |
| Compatibility | Additive. No existing key changes. |
| Client PR | FerrLab/airspace-acars#106 |
| Date | 2026-10-07 |

## Summary

Each report in the posted array now carries `runway` and `stand`, read from
the simulator's own airport scenery. Each says where on the airport the
aircraft is, and is `null` when the aircraft is not on a runway or not on a
stand.

The request body is still a JSON array of up to 100 reports, and every other
key is unchanged.

## Schema

Numbers use the same measurement wrapper as the rest of the report:
`{"value": <number|null>, "unit": "<string>"}`. `value` is `null` when the
simulator gave a non-finite number.

### `runway`: object or `null`

```jsonc
"runway": {
  "ends": [                       // always exactly 2
    {
      "designator": "09L",        // string, see "Designators"
      "latitude":  {"value": 51.4775,   "unit": "deg"},
      "longitude": {"value": -0.484961, "unit": "deg"},
      "altitude":  {"value": 79,        "unit": "ft"}   // feet above MSL
    },
    {
      "designator": "27R",
      "latitude":  {"value": 51.477661, "unit": "deg"},
      "longitude": {"value": -0.433128, "unit": "deg"},
      "altitude":  {"value": 78,        "unit": "ft"}
    }
  ]
}
```

### `stand`: object or `null`

```jsonc
"stand": {
  "name": "A12",                  // string, see "Stand names"
  "latitude":  {"value": 51.4712, "unit": "deg"},
  "longitude": {"value": -0.4598, "unit": "deg"}
}
```

### JSON Schema (draft 2020-12)

```json
{
  "$defs": {
    "measurement": {
      "type": "object",
      "required": ["value", "unit"],
      "properties": {
        "value": {"type": ["number", "null"]},
        "unit": {"type": "string"}
      }
    },
    "runwayEnd": {
      "type": "object",
      "required": ["designator", "latitude", "longitude", "altitude"],
      "properties": {
        "designator": {"type": "string"},
        "latitude":  {"$ref": "#/$defs/measurement"},
        "longitude": {"$ref": "#/$defs/measurement"},
        "altitude":  {"$ref": "#/$defs/measurement"}
      }
    }
  },
  "properties": {
    "runway": {
      "oneOf": [
        {"type": "null"},
        {
          "type": "object",
          "required": ["ends"],
          "properties": {
            "ends": {"type": "array", "items": {"$ref": "#/$defs/runwayEnd"}, "minItems": 2, "maxItems": 2}
          }
        }
      ]
    },
    "stand": {
      "oneOf": [
        {"type": "null"},
        {
          "type": "object",
          "required": ["name", "latitude", "longitude"],
          "properties": {
            "name": {"type": "string"},
            "latitude":  {"$ref": "#/$defs/measurement"},
            "longitude": {"$ref": "#/$defs/measurement"}
          }
        }
      ]
    }
  }
}
```

## Semantics

### When each field is set

- **`runway`** is set when the aircraft is **on the ground** and inside the
  runway's paved rectangle: between the two ends, and within half the runway
  width of the centreline.
  - Where runways cross, it is the runway the aircraft is lined up with.
  - An aircraft in the air is never on a runway, even a few feet over the
    threshold. Use `sensors.onGround` to tell airborne from on the ground.
- **`stand`** is set when the aircraft is **on the ground**, within the
  stand's radius, and moving **under 3 kt**. If stands overlap, it is the
  nearest one.
  - An aircraft taxiing past gates does not show up on each one.
  - A pushback (about 2 kt) still counts as on the stand.
- **The two are independent.** Both can be `null`, and in principle both can
  be set at once (scenery with a stand on the runway). Do not assume one
  excludes the other.
- **Missing key vs `null`:** a missing key means the same as `null`.
  - Older ACARS versions never send these keys.
  - Reports queued offline in a client's outbox before the upgrade are
    replayed later without them.

### Latency

The airport is looked up on the ground, or below 2,500 ft AGL on approach,
so the runway is normally known at touchdown. When the aircraft first appears
at an airport (for example a flight started at the gate), the first report or
two may be `null` while the lookup runs. It can take up to about 30 s.

Do not treat a single `null` between non-null values as a real change.

### Runway ends

- **Order:** the order the scenery gives. This is usually the lower number
  first, but don't rely on it. Match ends by `designator`.
- **Position:** the *physical* end of the pavement. This is **not** a
  displaced landing threshold.
- **Altitude**, in feet above mean sea level, depends on the simulator (see
  `simulator` at the top level of the report):

  | `simulator` | `altitude` of each end |
  |---|---|
  | `SimConnect` (MSFS 2020/2024) | Elevation of the **runway centre**. Both ends carry the same value. |
  | `X-Plane` | The landing-threshold elevation from the X-Plane navdata, a pilot's AIRAC first. Each end has its own value. With no navdata, both ends carry the airport elevation. |

### Designators

Designators follow charts:
- Two-digit number plus an optional `L`, `R`, `C`: `09L`, `27R`, `18C`, `36`.
- MSFS may also send `A`/`B` suffixes (`04A`), or compass names for
  unnumbered strips (`N`, `NE`, `E`, `SE`, `S`, `SW`, `W`, `NW`).
- X-Plane sends the designator exactly as written in the scenery, so treat it
  as free text.

### Stand names

- **MSFS:** built from the scenery's parking type and number.
  - `GATE_A` 12 is `A12`.
  - `N_PARKING` 5 is `N5`.
  - A plain gate or parking spot is its number alone, e.g. `22`.
  - A gate letter suffix is appended, e.g. `B4B`.
- **X-Plane:** the stand's name in the scenery, free text that may contain
  spaces (e.g. `Remote Stand 5`).
- **Matching:** the same real stand can have different names in different
  sceneries. For matching across pilots, normalise (trim, compare without
  case) or use the coordinates.

### Precision

- Positions come from the simulator scenery, not a real-world database. Small
  offsets of a few metres from published data are normal, and payware scenery
  can differ more.
- Stand coordinates are the stand's reference point, not the aircraft's
  position. The aircraft's own position is still in `position`.

## Examples

### Taxiing (neither)

```json
{ "...": "...", "runway": null, "stand": null }
```

### Parked at the gate

```json
{
  "...": "...",
  "sensors": {"onGround": true, "...": "..."},
  "runway": null,
  "stand": {
    "name": "A12",
    "latitude":  {"value": 51.4712, "unit": "deg"},
    "longitude": {"value": -0.4598, "unit": "deg"}
  }
}
```

### Take-off roll on 09L

```json
{
  "...": "...",
  "simulator": "SimConnect",
  "sensors": {"onGround": true, "...": "..."},
  "runway": {
    "ends": [
      {"designator": "09L",
       "latitude": {"value": 51.4775, "unit": "deg"},
       "longitude": {"value": -0.484961, "unit": "deg"},
       "altitude": {"value": 78.7, "unit": "ft"}},
      {"designator": "27R",
       "latitude": {"value": 51.477661, "unit": "deg"},
       "longitude": {"value": -0.433128, "unit": "deg"},
       "altitude": {"value": 78.7, "unit": "ft"}}
    ]
  },
  "stand": null
}
```

## Backend checklist

1. **Accept and store** `runway` and `stand` as nullable, and treat a missing
   key as null.
2. **Do not reject** a report because these keys are missing, `null`, or hold
   unexpected strings. The client retries rejected batches from its outbox, so
   a strict validator would hold up the flight's other data.
3. **Departure runway:** the `runway` of the last on-ground report before
   `sensors.onGround` turns false.
4. **Arrival runway:** the `runway` of the first on-ground reports after
   touchdown.
5. **Departure and arrival stands:** the first non-null `stand` of the flight,
   and the last one.
6. **Takeoff/landing runway heading:** compute it from the two ends,
   identifying the end the aircraft rolled towards with `attitude.headingTrue`.
   Do not rely on the order of `ends`.
7. **Ignore isolated nulls** between equal values (see "Latency").

## Not in this change

- No taxiway names.
- No runway while airborne (for example over the threshold on a go-around).
- No displaced-threshold positions; only the physical ends are sent.
- No airport ICAO on the runway or stand object. The airport is implied by
  position; `departure`/`arrival` still carry the booking ICAOs.
