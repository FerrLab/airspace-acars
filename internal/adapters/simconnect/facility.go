package simconnect

// Decoding of the SimConnect Facilities API replies that describe an airport's
// runways and parking spots.
//
// Everything here reads the dispatched message as little-endian bytes rather
// than casting it to a Go struct, for two reasons. The header changed between
// simulators: MSFS 2024 added a Type field to SIMCONNECT_RECV_FACILITY_DATA,
// and widened the airport ident from 6 to 9 characters. And the SDK does not
// document how those structs are packed around their bool and char fields. So
// only the fixed leading DWORDs are read by offset; a row's data is found by
// counting back from the end of the message, whose size the header states and
// whose payload size our own definition fixes. It is free of the Windows API,
// so it is tested on every platform.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"airspace-acars/internal/domain"
)

const (
	// recvFacilitiesListHeaderSize is SIMCONNECT_RECV (3 DWORDs) plus
	// dwRequestID, dwArraySize, dwEntryNumber and dwOutOf.
	recvFacilitiesListHeaderSize = 28

	// recvFacilityDataFixedSize is SIMCONNECT_RECV plus UserRequestId,
	// UniqueRequestId and ParentUniqueRequestId: the part of the header
	// whose layout no simulator version has changed.
	recvFacilityDataFixedSize = 24

	// The rest of the facility data header is IsListItem, ItemIndex and
	// ListSize, plus Type in MSFS 2024: 9 bytes packed, 16 padded. A payload
	// that would start outside this window means the message is not what we
	// think it is, and it is dropped rather than misread.
	minFacilityDataHeader = recvFacilityDataFixedSize + 9
	maxFacilityDataHeader = recvFacilityDataFixedSize + 16

	// facilityLatLonAltSize is the trailing LATITUDE, LONGITUDE, ALTITUDE
	// doubles of SIMCONNECT_DATA_FACILITY_AIRPORT, in every version.
	facilityLatLonAltSize = 24

	metresToFeet = 3.28084
)

// The fields requested for each object, in the order their values arrive.
// Payload sizes follow from the documented types: FLOAT64 is 8 bytes,
// FLOAT32, INT32 and UINT32 are 4.
var (
	airportFields = []string{"LATITUDE", "LONGITUDE", "ALTITUDE"}
	runwayFields  = []string{
		"LATITUDE", "LONGITUDE", "ALTITUDE", // FLOAT64
		"HEADING", "LENGTH", "WIDTH", // FLOAT32
		"PRIMARY_NUMBER", "PRIMARY_DESIGNATOR", // INT32
		"SECONDARY_NUMBER", "SECONDARY_DESIGNATOR", // INT32
	}
	parkingFields = []string{
		"NAME", "SUFFIX", "NUMBER", // INT32, INT32, UINT32
		"HEADING", "RADIUS", "BIAS_X", "BIAS_Z", // FLOAT32
	}
)

const (
	airportPayloadSize = 3 * 8
	runwayPayloadSize  = 3*8 + 3*4 + 4*4
	parkingPayloadSize = 3*4 + 4*4
)

// runwayDefinition and parkingDefinition are the two facility definitions.
// Runways and parking spots are asked for separately so that every row in a
// reply is either the airport (no parent) or one kind of child; telling two
// kinds of child apart would need the Type field that MSFS 2020 lacks.
func runwayDefinition() []string  { return facilityDefinition("RUNWAY", runwayFields) }
func parkingDefinition() []string { return facilityDefinition("TAXI_PARKING", parkingFields) }

func facilityDefinition(child string, fields []string) []string {
	def := []string{"OPEN AIRPORT"}
	def = append(def, airportFields...)
	def = append(def, "OPEN "+child)
	def = append(def, fields...)
	return append(def, "CLOSE "+child, "CLOSE AIRPORT")
}

// errMalformedFacility marks a facility message whose size does not fit what
// was asked for.
var errMalformedFacility = errors.New("malformed facility message")

// airportEntry is one airport in the simulator's facility cache.
type airportEntry struct {
	ICAO     string
	Lat, Lon float64
}

// airportListPage is one packet of a SIMCONNECT_RECV_AIRPORT_LIST reply.
type airportListPage struct {
	RequestID   uint32
	EntryNumber uint32
	OutOf       uint32
	Airports    []airportEntry
}

func u32(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }
func i32(b []byte, off int) int32  { return int32(binary.LittleEndian.Uint32(b[off:])) }
func f32(b []byte, off int) float64 {
	return float64(math.Float32frombits(binary.LittleEndian.Uint32(b[off:])))
}
func f64(b []byte, off int) float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(b[off:]))
}

// messageBytes returns the message as long as its header says it is.
func messageBytes(msg []byte) ([]byte, error) {
	if len(msg) < 12 {
		return nil, errMalformedFacility
	}
	size := int(u32(msg, 0))
	if size < 12 || size > len(msg) {
		return nil, fmt.Errorf("%w: size %d of %d bytes", errMalformedFacility, size, len(msg))
	}
	return msg[:size], nil
}

// decodeAirportList reads one airport list packet. Each entry's size is
// whatever the message says it is: the ident is a NUL-terminated string at
// its start, and the coordinates are its last three doubles.
func decodeAirportList(msg []byte) (airportListPage, error) {
	msg, err := messageBytes(msg)
	if err != nil {
		return airportListPage{}, err
	}
	if len(msg) < recvFacilitiesListHeaderSize {
		return airportListPage{}, errMalformedFacility
	}
	page := airportListPage{
		RequestID:   u32(msg, 12),
		EntryNumber: u32(msg, 20),
		OutOf:       u32(msg, 24),
	}
	count := int(u32(msg, 16))
	body := msg[recvFacilitiesListHeaderSize:]
	if count == 0 {
		return page, nil
	}
	if len(body)%count != 0 || len(body)/count <= facilityLatLonAltSize {
		return airportListPage{}, fmt.Errorf("%w: %d bytes for %d airports", errMalformedFacility, len(body), count)
	}
	size := len(body) / count
	for i := 0; i < count; i++ {
		e := body[i*size : (i+1)*size]
		at := size - facilityLatLonAltSize
		page.Airports = append(page.Airports, airportEntry{
			ICAO: cString(e[:at]),
			Lat:  f64(e, at),
			Lon:  f64(e, at+8),
		})
	}
	return page, nil
}

func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// facilityRow is one SIMCONNECT_RECV_FACILITY_DATA message: the airport
// itself (no parent) or one of its runways or parking spots.
type facilityRow struct {
	RequestID uint32
	IsChild   bool
	Payload   []byte
}

// decodeFacilityRow splits a facility data message into its header and the
// values of the object it carries. childSize is the payload size of the
// child kind the request asked for.
func decodeFacilityRow(msg []byte, childSize int) (facilityRow, error) {
	msg, err := messageBytes(msg)
	if err != nil {
		return facilityRow{}, err
	}
	if len(msg) < minFacilityDataHeader {
		return facilityRow{}, errMalformedFacility
	}
	row := facilityRow{RequestID: u32(msg, 12), IsChild: u32(msg, 20) != 0}
	size := airportPayloadSize
	if row.IsChild {
		size = childSize
	}
	start := len(msg) - size
	if start < minFacilityDataHeader || start > maxFacilityDataHeader {
		return facilityRow{}, fmt.Errorf("%w: %d-byte row for a %d-byte payload", errMalformedFacility, len(msg), size)
	}
	row.Payload = msg[start:]
	return row, nil
}

// airportRow is the airport's own values, in metres as SimConnect gives them.
type airportRow struct {
	Lat, Lon, AltM float64
}

func readAirportRow(p []byte) airportRow {
	return airportRow{Lat: f64(p, 0), Lon: f64(p, 8), AltM: f64(p, 16)}
}

// runwayRow is a runway as SimConnect describes it: by its centre.
type runwayRow struct {
	Lat, Lon, AltM         float64
	Heading, Length, Width float64
	PrimaryNumber          int32
	PrimaryDesignator      int32
	SecondaryNumber        int32
	SecondaryDesignator    int32
}

func readRunwayRow(p []byte) runwayRow {
	return runwayRow{
		Lat: f64(p, 0), Lon: f64(p, 8), AltM: f64(p, 16),
		Heading: f32(p, 24), Length: f32(p, 28), Width: f32(p, 32),
		PrimaryNumber: i32(p, 36), PrimaryDesignator: i32(p, 40),
		SecondaryNumber: i32(p, 44), SecondaryDesignator: i32(p, 48),
	}
}

// parkingRow is a parking spot, placed relative to the airport reference.
type parkingRow struct {
	Name, Suffix int32
	Number       uint32
	Heading      float64
	Radius       float64
	BiasX, BiasZ float64
}

func readParkingRow(p []byte) parkingRow {
	return parkingRow{
		Name: i32(p, 0), Suffix: i32(p, 4), Number: u32(p, 8),
		Heading: f32(p, 12), Radius: f32(p, 16), BiasX: f32(p, 20), BiasZ: f32(p, 24),
	}
}

// buildLayout turns the rows of the two replies into an airport layout.
//
// A runway's LENGTH runs threshold to threshold, displaced thresholds
// included and blast pads excluded, so half of it either way along the
// heading from the centre lands on the two physical ends. The primary end is
// the one whose number is the heading, so it lies behind the centre. Neither
// end has an elevation of its own in the facility data; both carry the
// runway centre's.
func buildLayout(icao string, ap airportRow, runways []runwayRow, parkings []parkingRow) *domain.AirportLayout {
	l := &domain.AirportLayout{
		ICAO: icao, RefLat: ap.Lat, RefLon: ap.Lon,
		ElevationFt: ap.AltM * metresToFeet,
	}
	for _, r := range runways {
		altFt := r.AltM * metresToFeet
		pLat, pLon := domain.Destination(r.Lat, r.Lon, r.Heading+180, r.Length/2)
		sLat, sLon := domain.Destination(r.Lat, r.Lon, r.Heading, r.Length/2)
		l.Runways = append(l.Runways, domain.Runway{
			Ends: [2]domain.RunwayEnd{
				{Designator: runwayDesignator(r.PrimaryNumber, r.PrimaryDesignator), Lat: pLat, Lon: pLon, AltFt: altFt},
				{Designator: runwayDesignator(r.SecondaryNumber, r.SecondaryDesignator), Lat: sLat, Lon: sLon, AltFt: altFt},
			},
			WidthM: r.Width,
		})
	}
	for _, p := range parkings {
		lat, lon := domain.OffsetMeters(ap.Lat, ap.Lon, p.BiasX, p.BiasZ)
		l.Stands = append(l.Stands, domain.Stand{
			Name: parkingName(p.Name, p.Suffix, p.Number),
			Lat:  lat, Lon: lon,
			RadiusM: p.Radius,
		})
	}
	return l
}

// runwayDesignator renders the RUNWAY NUMBER and DESIGNATOR enums the way a
// chart does: 9 and LEFT is "09L". Numbers 37–44 are the compass names some
// unnumbered strips use.
func runwayDesignator(number, designator int32) string {
	var s string
	switch {
	case number >= 1 && number <= 36:
		s = fmt.Sprintf("%02d", number)
	case number >= 37 && number <= 44:
		s = [...]string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}[number-37]
	}
	switch designator {
	case 1:
		s += "L"
	case 2:
		s += "R"
	case 3:
		s += "C"
	case 5:
		s += "A"
	case 6:
		s += "B"
	}
	return s
}

// parkingName renders the TAXI_PARKING NAME, NUMBER and SUFFIX enums as a
// stand name: GATE_A and 12 is "A12", N_PARKING and 5 is "N5", a plain GATE
// or PARKING is its number alone.
func parkingName(name, suffix int32, number uint32) string {
	prefix := ""
	switch {
	case name >= 2 && name <= 9:
		prefix = [...]string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}[name-2]
	case name >= 12 && name <= 37:
		prefix = string(rune('A' + name - 12))
	}
	s := fmt.Sprintf("%s%d", prefix, number)
	if suffix >= 12 && suffix <= 37 {
		s += string(rune('A' + suffix - 12))
	}
	return s
}

// nearestAirport picks the closest airport within maxM of the position.
func nearestAirport(airports []airportEntry, lat, lon, maxM float64) (airportEntry, bool) {
	best, bestM := airportEntry{}, math.Inf(1)
	for _, a := range airports {
		if a.ICAO == "" {
			continue
		}
		if d := domain.DistanceM(lat, lon, a.Lat, a.Lon); d < bestM {
			best, bestM = a, d
		}
	}
	return best, bestM <= maxM
}
