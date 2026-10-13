# Auto-start and auto-finish

The ACARS can start and finish a booked flight without the pilot pressing a
button. Each has its own switch under **Settings → Simulator**, both on by
default. Manual **Start** and **Finish** are never restricted by any of this.

## Auto-start

Fires once each time the aircraft becomes ready to depart:

- beacon on, at least one engine running, on the ground, stopped (under 1 kt);
- the pilot has a booking (`GET /api/v2/acars/booking`);
- the aircraft is **at the booking's departure airport**.

When the last condition fails, the flight is not started and the pilot sees
"Not starting automatically: the aircraft is not at SBGR". It is not retried
until the aircraft stops being ready (beacon or engines off) and becomes ready
again.

## Auto-finish

Fires once per shutdown, when all of:

- the flight has been in the air since it started, so a circuit (departure and
  arrival the same airport) does not finish at the gate before it flies;
- beacon off, every engine stopped, on the ground, stopped;
- the aircraft is **at the flight's current arrival airport**.

A shutdown anywhere else leaves the flight open and tells the pilot once:
"Not finishing automatically: the aircraft is not at SBRJ". If the booking then
reports a diversion to where the aircraft is parked, the flight finishes on the
next reading. Auto-finish goes through the same `FinishFlight` as the button:
the outbox is drained first, the one-minute minimum applies, and a failed finish
is not retried automatically.

## Where the aircraft is

`domain.AtAirport` counts the aircraft as at an airport when either:

- the simulator's own scenery (the ground locator's layout, see
  [runway-stand.md](runway-stand.md)) covers the aircraft's position and has
  that ICAO; or
- the aircraft is within **5 NM** (`domain.AutoFlightRadiusM`) of the
  coordinates the booking gives for the airport (`latitude`/`longitude` or
  `lat`/`lon`, numbers or strings). An airport the booking gives no
  coordinates for falls back to the built-in table used by the logbook.

With neither, the aircraft cannot be placed and nothing is automated.

## Diversions

The server records a diversion by setting `alternate_airport` on the booking,
and from then on that airport is the flight's arrival (the dashboard has always
shown it so). While a flight is active, the ACARS re-reads its booking every
30 seconds (`bookingRefreshEvery`). When the arrival changes:

- position reports, the finish request and the auto-finish check use the new
  arrival;
- the `flight-arrival-changed` event (payload: the new ICAO) updates the
  dashboard;
- `flight arrival changed` is logged at Info and `flight.arrival_changed` is
  counted.

A reply for a different booking, a booking with no arrival, or a failed request
keeps the last known arrival. The first failure of a streak is logged at Warn
(Debug when it is only the network), the rest at Debug. Each refresh is a
`booking.refresh` span.

## Events

| Event | Payload | Meaning |
|---|---|---|
| `auto-flight-start` | callsign | a flight is being started automatically |
| `auto-flight-finish` | arrival ICAO | a flight is being finished automatically |
| `auto-flight-skipped` | `{action: "start" \| "finish", airport}` | not started or finished: the aircraft is not at `airport` |
| `flight-arrival-changed` | arrival ICAO | the booking's arrival changed mid-flight |
