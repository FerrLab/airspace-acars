# Auto-Reconnect Stale Simulator Connections

## Problem

The simulator connection can become stale mid-flight — the UI appears connected but data stops updating. Users must manually disconnect and reconnect to recover. This interrupts active flights and position reporting.

**Root causes by adapter:**
- **SimConnect:** `GetFlightData()` returns cached `latestData` with no timestamp check. When SimConnect stops dispatching, the cached data is returned indefinitely with no error.
- **X-Plane:** Has a 3-second `lastReceived` timeout that detects staleness, but `dataStreamLoop` just logs the error and keeps polling the broken adapter forever — it never tears down and reconnects.

## Solution: Approach 1 — `LastReceived()` interface method + auto-reconnect

### Interface Change

Add `LastReceived() time.Time` to `SimConnector`. Both adapters track when they last received real data from the simulator.

### Staleness Detection

In `dataStreamLoop`, after each poll: if `simActive` is true and `time.Since(connector.LastReceived()) > 10s`, trigger a reconnect.

10 seconds is generous enough to cover loading screens and brief sim pauses, but short enough to recover quickly.

### Reconnect Flow

New `reconnectSim(adapterName)` method on `FlightDataService`:
1. Disconnect old adapter (best-effort)
2. Create new adapter of the same type
3. Connect it
4. Swap into `f.connector`
5. Set `simActive = false` (wait for first data to confirm)

### Backoff

Track consecutive failures. Delay = `min(2^attempts * 5s, 60s)`. Reset on success. Prevents hammering when the sim is genuinely closed.

### State Fields Added to FlightDataService

- `reconnectAttempts int`
- `lastReconnectAt time.Time`
- `adapterName string`

### Edge Cases

- **User manually disconnects:** `DisconnectSim()` sets connector=nil, stream stops. No conflict.
- **Sim genuinely closed:** Reconnect fails, backoff prevents hammering. User sees disconnected state.
- **Loading screens:** Most sims still dispatch during loads. 10s threshold covers pauses.
- **Race with flight position reporting:** `positionLoop` already handles `GetFlightDataNow()` errors gracefully.

### Files Modified

| File | Change |
|------|--------|
| `sim_connector.go` | Add `LastReceived() time.Time` to interface |
| `simconnect_adapter_windows.go` | Add `lastReceived` field, update in `run()`, add method |
| `xplane_adapter.go` | Add `LastReceived()` method (field exists) |
| `flight_data_service.go` | Add reconnect fields, `reconnectSim()`, staleness check in `dataStreamLoop` |
| `mock_sim_connector_test.go` | Add `LastReceived()` to mock |

### Not Modified

- `flight_service.go` — flight state untouched
- Frontend — already handles `connection-state` events
- Settings — no new user-facing configuration
