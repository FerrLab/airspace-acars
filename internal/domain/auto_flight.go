package domain

import "strings"

// AutoFlightRadiusM is how far from an airport's published coordinates the
// aircraft may be and still count as at that airport for a flight to start
// or finish on its own. Five nautical miles reaches the remotest stands of
// the largest airports, measured from a reference point that can sit
// anywhere on the field, while still telling apart most pairs of
// neighbouring airports. Two fields closer than this (a few city pairs are)
// both count; the simulator's own scenery, when it can be read, settles
// those first.
const AutoFlightRadiusM = 5 * 1852.0

// stationaryKt is the ground speed below which the aircraft counts as
// stopped. It matches the 1 kt StartFlight demands, so an auto-start never
// asks for a start that StartFlight then refuses.
const stationaryKt = 1.0

// AutoFlightSkip tells the pilot a flight was not started or finished on its
// own because the aircraft is not at the airport the booking names.
type AutoFlightSkip struct {
	// Action is "start" or "finish".
	Action string `json:"action"`
	// Airport is the ICAO of the airport the aircraft was expected at.
	Airport string `json:"airport"`
}

// ReadyToDepart reports whether the aircraft looks like it is about to leave
// the gate: beacon on, at least one engine running, on the ground and
// stopped. It is the trigger for an automatic start.
func ReadyToDepart(fd *FlightData) bool {
	if fd == nil {
		return false
	}
	running := false
	for _, e := range fd.Engines {
		if e.Running {
			running = true
			break
		}
	}
	return fd.Lights.Beacon && running && fd.Sensors.OnGround && fd.Attitude.GS < stationaryKt
}

// ShutDown reports whether the aircraft is parked and shut down: beacon off,
// every engine stopped, on the ground and stopped. It is the trigger for an
// automatic finish.
func ShutDown(fd *FlightData) bool {
	if fd == nil {
		return false
	}
	for _, e := range fd.Engines {
		if e.Running {
			return false
		}
	}
	return !fd.Lights.Beacon && fd.Sensors.OnGround && fd.Attitude.GS < stationaryKt
}

// AtAirport reports whether the aircraft at (lat, lon) is at airport.
//
// simAirport is the ICAO of the field the simulator's own scenery puts the
// aircraft on, or empty when it puts it on none or cannot say. A match there
// settles it. Otherwise the aircraft must be within AutoFlightRadiusM of the
// airport's coordinates. An airport with neither is not one the aircraft can
// be shown to be at, so the answer is no.
func AtAirport(lat, lon float64, airport AirportInfo, simAirport string) bool {
	icao := strings.TrimSpace(airport.ICAO)
	if icao == "" {
		return false
	}
	if simAirport != "" && strings.EqualFold(strings.TrimSpace(simAirport), icao) {
		return true
	}
	if airport.Latitude == 0 && airport.Longitude == 0 {
		return false
	}
	return DistanceM(lat, lon, airport.Latitude, airport.Longitude) <= AutoFlightRadiusM
}
