# Aircraft profiles

An aircraft profile is a JSON document that tells the ACARS two things:

1. **matcher** — which aircraft it applies to, as a boolean expression over the
   loaded aircraft's identity;
2. **masher** — how one or more data points should be collected for that
   aircraft, instead of the way the simulator adapter collects them by default.

Add-ons rarely drive every stock simulation variable. The Fenix A320 runs its
FCU on its own local variables, the FSLabs gear follows animation variables, and
X-Plane reports 8.33 kHz radio channels on a different dataref from the 25 kHz
one. A profile states those differences in data, so support for a new add-on is
a JSON file rather than a code change.

Profiles live in `internal/profiles/builtin/` (shipped with the app) and in
`<user config>/airspace-acars/profiles/` (yours). A file in your directory that
reuses a built-in `id` replaces it, so any shipped profile can be overridden
without touching the application.

## Anatomy

```json
{
  "id": "fenix-a32x",
  "name": "Fenix A318/A319/A320/A321",
  "description": "Reads the FCU from the Fenix local variables.",
  "priority": 100,
  "match": {
    "all": [
      { "field": "simulator", "op": "equals", "value": "simconnect" },
      { "field": "aircraftName", "op": "contains", "value": "fenix" }
    ]
  },
  "mash": {
    "autopilot.master": {
      "reduce": "or",
      "bindings": [
        { "source": { "kind": "lvar", "name": "I_FCU_AP1" }, "transform": [{ "op": "bool" }] },
        { "source": { "kind": "lvar", "name": "I_FCU_AP2" }, "transform": [{ "op": "bool" }] },
        { "source": { "kind": "simvar", "name": "AUTOPILOT MASTER", "unit": "Bool" }, "transform": [{ "op": "bool" }] }
      ]
    }
  }
}
```

| Field | Meaning |
|---|---|
| `id` | Unique identifier. A user file with the same `id` replaces the built-in. |
| `name` | Shown in the UI and in logs. |
| `description`, `notes` | Free text. `notes` is where to record what still needs verifying. |
| `priority` | Higher wins when two profiles mash the same data point. Default `0`. |
| `disabled` | `true` keeps the file but never matches. |
| `match` | The selector. Omitted means "every aircraft". |
| `mash` | Data point ID → how to collect it. |

## Matching

A match node is either a **group** or a **condition**, and groups nest freely.

```json
{ "all": [ … ] }   every child must match
{ "any": [ … ] }   at least one child must match
{ "not": { … } }   the child must not match
```

`all`, `any` and `not` can appear together in one node; the node matches when
all of them are satisfied.

A condition reads one field and applies one operator:

| Field | Value |
|---|---|
| `aircraftName` | The simulator title — MSFS `TITLE`, X-Plane `acf_descrip` |
| `aircraftType` | ICAO type — MSFS `ATC MODEL`, X-Plane `acf_ICAO` |
| `simulator` | `simconnect` or `xplane` |
| `engineCount` | Number of engines fitted |

| Operator | Notes |
|---|---|
| `equals`, `notEquals` | |
| `contains`, `notContains` | |
| `startsWith`, `endsWith` | |
| `matches` | Go regular expression |
| `in`, `notIn` | `value` is an array |
| `greaterThan`, `lessThan` | Numeric |
| `exists` | Field is present and non-empty |

String comparisons ignore case unless the condition sets
`"caseSensitive": true`. Aircraft titles vary by livery, so case-insensitive
`contains` is usually what you want.

### Layering

Every profile that matches is applied, from the lowest `priority` to the
highest; ties break in favour of the more specific selector (more leaf
conditions), then by `id`. A higher-priority profile overrides a lower one only
on the data points they share. That is how the shipped Fenix profiles work: one
family profile carries the systems bindings, and a small variant profile per
airframe adds nothing but the ICAO type.

## Mashing

Each entry of `mash` maps a data point ID to its bindings. Three shapes are
accepted:

```json
"autopilot.master": { "source": … }                         one binding
"autopilot.master": [ { "source": … }, { "source": … } ]    candidates, first usable wins
"autopilot.master": { "reduce": "or", "bindings": [ … ] }   all usable, combined
```

Without `reduce`, the bindings are **candidates**: the first one whose `sim`
scope matches and whose source kind the active adapter can read is used, and the
rest are the fallback chain. With `reduce`, every usable binding is read and the
values are combined: `or`, `and`, `max`, `min`, `sum`.

A binding is:

```json
{ "sim": "simconnect", "source": { … }, "transform": [ … ] }
```

`sim` restricts the binding to one simulator; omit it to allow both.

### Sources

| `kind` | Reads | Fields |
|---|---|---|
| `simvar` | An MSFS simulation variable (`A:`) | `name`, `unit` |
| `lvar` | An MSFS local panel variable (`L:`) | `name` |
| `dataref` | An X-Plane dataref | `name` |
| `const` | A fixed value from the profile | `value` (number, string or boolean) |

`const` is how a profile corrects an add-on's reported ICAO type — the Fenix
A321neo reports `A21N` whatever the title says.

**Local variables are not readable yet.** SimConnect cannot read `L:` vars on
its own; that needs a WASM bridge inside the simulator, which is not wired up.
Until it is, a binding with `"kind": "lvar"` is skipped and the next candidate
is used, which is why every shipped `lvar` binding is followed by the stock
simulation variable. When the bridge lands, those profiles start using the
local variables with no change to the files. A data point where every candidate
was unusable is listed in the plan's `skipped`, and the adapter's own reading is
left in place.

### Transforms

A transform is a pipeline: each step takes the number the previous one produced.
Comparison steps return `1` or `0`, so they can feed boolean data points.

| Op | Arguments | Result |
|---|---|---|
| `scale`, `offset`, `divide` | `value` | Arithmetic |
| `round` | `value` (decimals, optional) | Nearest integer, or to N decimals |
| `floor`, `ceil`, `abs` | | |
| `clamp` | `min`, `max` | Constrained to the range |
| `bool` | `value` (threshold, optional) | 1 when non-zero, or when ≥ threshold |
| `not` | | 1 when the input is zero |
| `eq`, `ne`, `gt`, `gte`, `lt`, `lte` | `value` | 1 or 0 |
| `map` | `map`, `default` | Table lookup keyed by the input value |

`map` is how a handle index becomes a percentage:

```json
{ "op": "map", "map": { "0": 0, "1": 25, "2": 50, "3": 75, "4": 100 }, "default": 0 }
```

## Data points

| ID | Type | Description |
|---|---|---|
| `aircraft.name` | string | Aircraft title reported to the network |
| `aircraft.type` | string | ICAO type reported to the network |
| `altimeter` | float | Altimeter setting in inHg |
| `apu.genActive` | bool | APU generator supplying power |
| `apu.genSwitch` | bool | APU generator switch on |
| `apu.rpmPercent` | float | APU RPM percent |
| `apu.switchOn` | bool | APU master switch on |
| `attitude.gForce` | float | Normal load factor in G |
| `attitude.gs` | float | Ground speed in knots |
| `attitude.headingMag` | float | Magnetic heading in degrees |
| `attitude.headingTrue` | float | True heading in degrees |
| `attitude.ias` | float | Indicated airspeed in knots |
| `attitude.pitch` | float | Pitch in degrees |
| `attitude.roll` | float | Bank in degrees |
| `attitude.tas` | float | True airspeed in knots |
| `attitude.vs` | float | Vertical speed in feet per minute |
| `autopilot.altitude` | float | Selected altitude in feet |
| `autopilot.approachHold` | bool | Approach mode armed or engaged |
| `autopilot.heading` | float | Selected heading in degrees |
| `autopilot.master` | bool | Autopilot engaged |
| `autopilot.navLock` | bool | Lateral navigation engaged |
| `autopilot.speed` | float | Selected airspeed in knots |
| `autopilot.vs` | float | Selected vertical speed in feet per minute |
| `controls.aileron` | float | Aileron position |
| `controls.elevator` | float | Elevator position |
| `controls.flaps` | float | Flap handle percent |
| `controls.gearDown` | bool | Gear handle down |
| `controls.rudder` | float | Rudder position |
| `controls.spoilers` | float | Spoiler handle percent |
| `doors.1.openRatio` | float | Door 1 open ratio (0-1) |
| `doors.2.openRatio` | float | Door 2 open ratio (0-1) |
| `doors.3.openRatio` | float | Door 3 open ratio (0-1) |
| `doors.4.openRatio` | float | Door 4 open ratio (0-1) |
| `doors.5.openRatio` | float | Door 5 open ratio (0-1) |
| `engines.1.exists` | bool | Engine 1 is fitted |
| `engines.1.mixture` | float | Engine 1 mixture lever percent |
| `engines.1.n1` | float | Engine 1 N1 percent |
| `engines.1.n2` | float | Engine 1 N2 percent |
| `engines.1.prop` | float | Engine 1 propeller lever percent |
| `engines.1.running` | bool | Engine 1 combustion |
| `engines.1.throttle` | float | Engine 1 throttle lever percent |
| `engines.2.exists` | bool | Engine 2 is fitted |
| `engines.2.mixture` | float | Engine 2 mixture lever percent |
| `engines.2.n1` | float | Engine 2 N1 percent |
| `engines.2.n2` | float | Engine 2 N2 percent |
| `engines.2.prop` | float | Engine 2 propeller lever percent |
| `engines.2.running` | bool | Engine 2 combustion |
| `engines.2.throttle` | float | Engine 2 throttle lever percent |
| `engines.3.exists` | bool | Engine 3 is fitted |
| `engines.3.mixture` | float | Engine 3 mixture lever percent |
| `engines.3.n1` | float | Engine 3 N1 percent |
| `engines.3.n2` | float | Engine 3 N2 percent |
| `engines.3.prop` | float | Engine 3 propeller lever percent |
| `engines.3.running` | bool | Engine 3 combustion |
| `engines.3.throttle` | float | Engine 3 throttle lever percent |
| `engines.4.exists` | bool | Engine 4 is fitted |
| `engines.4.mixture` | float | Engine 4 mixture lever percent |
| `engines.4.n1` | float | Engine 4 N1 percent |
| `engines.4.n2` | float | Engine 4 N2 percent |
| `engines.4.prop` | float | Engine 4 propeller lever percent |
| `engines.4.running` | bool | Engine 4 combustion |
| `engines.4.throttle` | float | Engine 4 throttle lever percent |
| `engines.count` | float | Number of engines fitted (sets every engine's exists flag) |
| `lights.beacon` | bool | Beacon light on |
| `lights.landing` | bool | Landing lights on |
| `lights.strobe` | bool | Strobe lights on |
| `position.altitude` | float | Indicated altitude in feet |
| `position.altitudeAGL` | float | Height above ground in feet |
| `position.latitude` | float | Latitude in degrees |
| `position.longitude` | float | Longitude in degrees |
| `qnh` | float | Sea level pressure in millibars |
| `radios.com1` | float | COM1 active frequency in MHz |
| `radios.com2` | float | COM2 active frequency in MHz |
| `radios.nav1` | float | NAV1 active frequency in MHz |
| `radios.nav1OBS` | float | NAV1 OBS course in degrees |
| `radios.nav2` | float | NAV2 active frequency in MHz |
| `radios.nav2OBS` | float | NAV2 OBS course in degrees |
| `radios.xpdrCode` | float | Transponder code |
| `radios.xpdrState` | float | Transponder mode (0 off, 1 standby, 2+ active) |
| `sensors.crashed` | bool | Crash flag set |
| `sensors.onGround` | bool | Aircraft is on the ground |
| `sensors.overspeedWarning` | bool | Overspeed warning active |
| `sensors.paused` | bool | Simulation paused |
| `sensors.simulationRate` | float | Simulation rate multiplier |
| `sensors.slew` | bool | Slew mode active |
| `sensors.stallWarning` | bool | Stall warning active |
| `simTime.localTime` | float | Local time in seconds |
| `simTime.zuluDay` | float | Zulu day of month |
| `simTime.zuluMonth` | float | Zulu month of year |
| `simTime.zuluTime` | float | Zulu time in seconds |
| `simTime.zuluYear` | float | Zulu year |
| `weight.fuel` | float | Fuel weight in pounds |
| `weight.total` | float | Total weight in pounds |
| `wind.direction` | float | Ambient wind direction in degrees |
| `wind.speed` | float | Ambient wind speed in knots |

`bool` points store true when the transformed value is non-zero. `string` points
can only be fed by a `const` source. `engines.count` is a shorthand that sets
every engine's `exists` flag at once.

## How a profile reaches the data

1. The adapter reads its built-in variable set and reports the aircraft's raw
   identity — the title and ICAO type *before* any profile rewrites them, so a
   profile that mashes `aircraft.type` cannot change what it matches against.
2. Once per second the application layer re-resolves the profile whenever the
   aircraft, the simulator or the profile setting has changed, and hands the
   resulting plan to the adapter.
3. The adapter subscribes to the variables the plan asks for — a second
   SimConnect data definition on MSFS, extra RREF subscriptions on X-Plane.
4. Every snapshot handed to the rest of the application has the plan applied on
   top of the adapter's own readings. A data point whose variable has not
   arrived yet keeps the adapter's value, so a profile can never blank a field.

## Settings

`aircraftProfile` in `settings.json`:

| Value | Behaviour |
|---|---|
| `auto` (default) | Match profiles against the loaded aircraft |
| `off` | Ignore profiles entirely |
| A profile `id` | Pin that profile, ignoring its selector |

Pinning is useful when the ACARS cannot identify the aircraft — X-Plane reports
the aircraft description character by character and some builds do not serve it
at all.

## Writing your own

Drop a `.json` file into `<user config>/airspace-acars/profiles/`
(`%APPDATA%\airspace-acars\profiles` on Windows) and use `ReloadProfiles` from
the UI, or restart the app. A file that fails to parse or validate is skipped
with a message in the log — one bad profile never stops the others from loading.
Unknown data point IDs, source kinds, transform ops, match fields and match
operators are all rejected at load time rather than silently ignored.
