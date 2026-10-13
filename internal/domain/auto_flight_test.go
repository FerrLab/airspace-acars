package domain

import "testing"

// SBGR's published reference point, and SBSP, 10 NM away across São Paulo.
var (
	sbgr = AirportInfo{ICAO: "SBGR", Latitude: -23.435556, Longitude: -46.473056}
	sbsp = AirportInfo{ICAO: "SBSP", Latitude: -23.626111, Longitude: -46.656389}
)

// The aircraft is at an airport when the simulator's scenery says so or when
// it is within the radius of the booking's coordinates, and at no airport
// that neither source can place it at.
func TestAtAirportPlacesTheAircraftFromEitherSource(t *testing.T) {
	// A remote stand about 3 km from SBGR's reference point.
	gateLat, gateLon := Destination(sbgr.Latitude, sbgr.Longitude, 90, 3000)
	edgeLat, edgeLon := Destination(sbgr.Latitude, sbgr.Longitude, 0, AutoFlightRadiusM-10)

	tests := []struct {
		name       string
		lat, lon   float64
		airport    AirportInfo
		simAirport string
		want       bool
	}{
		{"a stand within the radius", gateLat, gateLon, sbgr, "", true},
		{"the neighbouring airport", sbsp.Latitude, sbsp.Longitude, sbgr, "", false},
		{"just inside the radius", edgeLat, edgeLon, sbgr, "", true},
		{"the scenery names the airport", sbsp.Latitude, sbsp.Longitude, AirportInfo{ICAO: "SBGR"}, "sbgr", true},
		{"the scenery names another airport, coordinates agree", gateLat, gateLon, sbgr, "SBSP", true},
		{"no coordinates and no scenery", gateLat, gateLon, AirportInfo{ICAO: "SBGR"}, "", false},
		{"no coordinates, scenery names another airport", gateLat, gateLon, AirportInfo{ICAO: "SBGR"}, "SBSP", false},
		{"no airport at all", gateLat, gateLon, AirportInfo{}, "", false},
		{"no airport, whatever the scenery says", gateLat, gateLon, AirportInfo{}, "SBGR", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AtAirport(tt.lat, tt.lon, tt.airport, tt.simAirport); got != tt.want {
				t.Errorf("AtAirport = %v, want %v", got, tt.want)
			}
		})
	}
}

// Ten metres past the radius is out: an airport across town must not count.
func TestAtAirportStopsAtTheRadius(t *testing.T) {
	lat, lon := Destination(sbgr.Latitude, sbgr.Longitude, 180, AutoFlightRadiusM+10)
	if AtAirport(lat, lon, sbgr, "") {
		t.Error("an aircraft past the radius counts as at the airport; a flight would start or finish at the wrong field")
	}
}

func parkedAt(beacon bool, running ...bool) *FlightData {
	fd := &FlightData{}
	fd.Lights.Beacon = beacon
	fd.Sensors.OnGround = true
	for i, r := range running {
		fd.Engines[i].Exists = true
		fd.Engines[i].Running = r
	}
	return fd
}

// Auto-start fires as the aircraft is about to leave the gate: beacon on, an
// engine running, still stopped on the ground.
func TestReadyToDepart(t *testing.T) {
	moving := parkedAt(true, true, false)
	moving.Attitude.GS = 5
	airborne := parkedAt(true, true, true)
	airborne.Sensors.OnGround = false

	tests := []struct {
		name string
		fd   *FlightData
		want bool
	}{
		{"beacon on, one engine running", parkedAt(true, true, false), true},
		{"beacon off", parkedAt(false, true, true), false},
		{"no engine running", parkedAt(true, false, false), false},
		{"taxiing", moving, false},
		{"airborne", airborne, false},
		{"no data", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadyToDepart(tt.fd); got != tt.want {
				t.Errorf("ReadyToDepart = %v, want %v", got, tt.want)
			}
		})
	}
}

// Auto-finish fires once the aircraft is parked and shut down: beacon off,
// every engine stopped, stopped on the ground.
func TestShutDown(t *testing.T) {
	rolling := parkedAt(false, false, false)
	rolling.Attitude.GS = 3

	tests := []struct {
		name string
		fd   *FlightData
		want bool
	}{
		{"beacon and engines off", parkedAt(false, false, false), true},
		{"one engine still running", parkedAt(false, false, true), false},
		{"beacon still on", parkedAt(true, false, false), false},
		{"still rolling", rolling, false},
		{"no data", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShutDown(tt.fd); got != tt.want {
				t.Errorf("ShutDown = %v, want %v", got, tt.want)
			}
		})
	}
}
