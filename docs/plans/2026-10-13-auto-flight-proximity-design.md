# Auto-start and auto-finish only at the booked airports

## Goal

Auto-start fired wherever the aircraft happened to be once the beacon and an
engine were on, and auto-finish was removed in March for the mirror-image
reason: it ended flights wherever the engines were shut down. A flight should
start on its own only at its departure airport and finish on its own only at
its arrival airport, and the arrival airport is whatever the booking says now,
which changes when the pilot diverts to an alternate.

## Approach

- **Where is the aircraft?** `domain.AtAirport` says the aircraft is at an
  airport when the simulator's own scenery puts it on that field (the ground
  locator's layout covers the position and has that ICAO), or when it is
  within `AutoFlightRadiusM` (5 NM) of the coordinates the booking gives for
  it. Either source alone is enough, so a tenant whose airports carry no
  coordinates still works on a simulator with readable scenery, and X-Plane on
  another machine still works when the tenant has coordinates. With neither,
  the answer is no: nothing is automated that cannot be placed.
- **Auto-start** keeps its trigger (beacon on, an engine running, on the
  ground, stationary) and, after fetching the booking, also requires the
  aircraft to be at the booking's departure airport. When it is not, the
  pilot is told once and can still start by hand.
- **Auto-finish** comes back behind its own setting (default on), and fires
  only when all of: the flight has been airborne since it started, the
  aircraft is on the ground and stationary with the beacon and every engine
  off, and it is at the flight's current arrival airport. A shutdown anywhere
  else is reported once and left alone. "Has been airborne" keeps a
  circuit flight, whose departure and arrival are the same field, from
  finishing at the gate before it ever took off.
- **The booking is re-read every 30 s** while a flight is active, by a
  `bookingWatcher` started with the flight and stopped when it ends. When the
  arrival changes (the server sets `alternate_airport` on a diversion), the
  flight's arrival is updated, so position reports, the finish request, the
  dashboard and the auto-finish check all follow it, and the UI is told with
  `flight-arrival-changed`. A reply for another booking is ignored.
- The decision is a small `autoFlight` type that replaces the bare
  `autoStartArmed` field on `App`; it has its own lock because a reconnect
  can briefly run the next data stream beside the last.

## Span points

- `booking.refresh` around each watcher fetch; network failures are
  `Expected`, logged at Debug, and the last known arrival is kept.
- `flight.start` and `flight.finish` are unchanged and cover the automated
  calls too.

## Out of scope

- Manual Start and Finish are not restricted by position.
- Changing the departure airport mid-flight.
- The `map[string]interface{}` return of `GetBooking` (known debt).
