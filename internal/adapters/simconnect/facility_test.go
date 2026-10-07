package simconnect

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"airspace-acars/internal/domain"
)

// msgBuilder writes a SimConnect message the way the simulator lays it out:
// little-endian, with the total size in the first DWORD.
type msgBuilder struct{ b []byte }

func (m *msgBuilder) u32(v uint32) *msgBuilder {
	m.b = binary.LittleEndian.AppendUint32(m.b, v)
	return m
}
func (m *msgBuilder) i32(v int32) *msgBuilder { return m.u32(uint32(v)) }
func (m *msgBuilder) f32(v float64) *msgBuilder {
	return m.u32(math.Float32bits(float32(v)))
}
func (m *msgBuilder) f64(v float64) *msgBuilder {
	m.b = binary.LittleEndian.AppendUint64(m.b, math.Float64bits(v))
	return m
}
func (m *msgBuilder) raw(b ...byte) *msgBuilder { m.b = append(m.b, b...); return m }
func (m *msgBuilder) done() []byte {
	binary.LittleEndian.PutUint32(m.b, uint32(len(m.b)))
	return m.b
}

func recvHeader(id uint32) *msgBuilder {
	return (&msgBuilder{}).u32(0).u32(1).u32(id)
}

// airportListMsg builds an airport list packet whose idents are identLen
// bytes, as MSFS 2020 (6) and 2024 (9) send them.
func airportListMsg(identLen int, entry, outOf uint32, airports ...airportEntry) []byte {
	m := recvHeader(18).u32(7).u32(uint32(len(airports))).u32(entry).u32(outOf)
	for _, a := range airports {
		ident := make([]byte, identLen)
		copy(ident, a.ICAO)
		m.raw(ident...).raw('E', 'G', 0).f64(a.Lat).f64(a.Lon).f64(25)
	}
	return m.done()
}

// The airport list must read the same whichever simulator sent it.
func TestTheAirportListReadsTheSameFromBothSimulators(t *testing.T) {
	airports := []airportEntry{
		{ICAO: "EGLL", Lat: 51.4775, Lon: -0.4614},
		{ICAO: "EGLC", Lat: 51.5053, Lon: 0.0553},
	}
	for name, identLen := range map[string]int{"MSFS 2020": 6, "MSFS 2024": 9} {
		t.Run(name, func(t *testing.T) {
			page, err := decodeAirportList(airportListMsg(identLen, 0, 1, airports...))
			if err != nil {
				t.Fatal(err)
			}
			if page.RequestID != 7 || page.OutOf != 1 || len(page.Airports) != 2 {
				t.Fatalf("page = %+v", page)
			}
			for i, want := range airports {
				if page.Airports[i] != want {
					t.Errorf("airport %d = %+v, want %+v", i, page.Airports[i], want)
				}
			}
		})
	}
}

func TestAMangledAirportListIsRejected(t *testing.T) {
	good := airportListMsg(9, 0, 1, airportEntry{ICAO: "EGLL"})
	cases := map[string][]byte{
		"truncated header":       good[:20],
		"size beyond the buffer": append([]byte(nil), good[:len(good)-4]...),
		"entries that do not divide": func() []byte {
			b := append([]byte(nil), good...)
			binary.LittleEndian.PutUint32(b[16:], 3)
			return b
		}(),
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeAirportList(msg); !errors.Is(err, errMalformedFacility) {
				t.Fatalf("err = %v, want errMalformedFacility", err)
			}
		})
	}
}

// facilityDataMsg builds a SIMCONNECT_RECV_FACILITY_DATA message with the
// header of the given simulator around payload.
func facilityDataMsg(sim2024 bool, requestID, parent uint32, payload []byte) []byte {
	m := recvHeader(29).u32(requestID).u32(42).u32(parent)
	if sim2024 {
		m.u32(1) // Type
	}
	m.raw(1).u32(0).u32(1) // IsListItem, ItemIndex, ListSize, packed
	return m.raw(payload...).done()
}

func runwayPayload(r runwayRow) []byte {
	return (&msgBuilder{}).f64(r.Lat).f64(r.Lon).f64(r.AltM).
		f32(r.Heading).f32(r.Length).f32(r.Width).
		i32(r.PrimaryNumber).i32(r.PrimaryDesignator).
		i32(r.SecondaryNumber).i32(r.SecondaryDesignator).b
}

func parkingPayload(p parkingRow) []byte {
	return (&msgBuilder{}).i32(p.Name).i32(p.Suffix).u32(p.Number).
		f32(p.Heading).f32(p.Radius).f32(p.BiasX).f32(p.BiasZ).b
}

// A runway row is found by its size whichever header surrounds it.
func TestFacilityRowsReadTheSameFromBothSimulators(t *testing.T) {
	want := runwayRow{Lat: 51.4776, Lon: -0.4590, AltM: 24, Heading: 89.7, Length: 3902, Width: 50,
		PrimaryNumber: 9, PrimaryDesignator: 1, SecondaryNumber: 27, SecondaryDesignator: 2}
	for name, is2024 := range map[string]bool{"MSFS 2020": false, "MSFS 2024": true} {
		t.Run(name, func(t *testing.T) {
			airport, err := decodeFacilityRow(facilityDataMsg(is2024, 5, 0,
				(&msgBuilder{}).f64(51.4775).f64(-0.4614).f64(25).b), runwayPayloadSize)
			if err != nil {
				t.Fatal(err)
			}
			if airport.IsChild || readAirportRow(airport.Payload).Lat != 51.4775 {
				t.Fatalf("airport row read as %+v", airport)
			}

			row, err := decodeFacilityRow(facilityDataMsg(is2024, 5, 42, runwayPayload(want)), runwayPayloadSize)
			if err != nil {
				t.Fatal(err)
			}
			got := readRunwayRow(row.Payload)
			if !row.IsChild || row.RequestID != 5 || got.PrimaryNumber != 9 || got.SecondaryDesignator != 2 ||
				got.Lat != want.Lat || math.Abs(got.Length-3902) > 0.01 {
				t.Fatalf("runway row read as %+v (%+v)", got, row)
			}
		})
	}
}

// A row too short or too long for what was asked for is dropped, not read
// from the wrong offset.
func TestAFacilityRowOfTheWrongSizeIsRejected(t *testing.T) {
	parking := facilityDataMsg(true, 6, 42, parkingPayload(parkingRow{Name: 12, Number: 1}))
	if _, err := decodeFacilityRow(parking, runwayPayloadSize); !errors.Is(err, errMalformedFacility) {
		t.Fatalf("a parking row decoded as a runway: err = %v", err)
	}
	if _, err := decodeFacilityRow(parking[:20], parkingPayloadSize); !errors.Is(err, errMalformedFacility) {
		t.Fatalf("a truncated row decoded: err = %v", err)
	}
}

// The definitions must open and close in pairs, or SimConnect rejects them.
func TestTheFacilityDefinitionsAreBalanced(t *testing.T) {
	for name, def := range map[string][]string{"runway": runwayDefinition(), "parking": parkingDefinition()} {
		depth := 0
		for _, f := range def {
			switch {
			case len(f) > 5 && f[:5] == "OPEN ":
				depth++
			case len(f) > 6 && f[:6] == "CLOSE ":
				depth--
			}
			if depth < 0 {
				t.Fatalf("%s definition closes before it opens: %v", name, def)
			}
		}
		if depth != 0 {
			t.Fatalf("%s definition leaves %d object(s) open: %v", name, depth, def)
		}
	}
}

// A runway is described by its centre; the report wants its two ends.
func TestARunwayIsTurnedIntoItsTwoEnds(t *testing.T) {
	centreLat, centreLon := 51.477580, -0.459045
	l := buildLayout("EGLL", airportRow{Lat: 51.4775, Lon: -0.4614, AltM: 25},
		[]runwayRow{{Lat: centreLat, Lon: centreLon, AltM: 24, Heading: 89.7, Length: 3600, Width: 50,
			PrimaryNumber: 9, PrimaryDesignator: 1, SecondaryNumber: 27, SecondaryDesignator: 2}},
		nil)

	if len(l.Runways) != 1 {
		t.Fatalf("%d runways, want 1", len(l.Runways))
	}
	r := l.Runways[0]
	if r.Ends[0].Designator != "09L" || r.Ends[1].Designator != "27R" {
		t.Fatalf("ends are %s and %s, want 09L and 27R", r.Ends[0].Designator, r.Ends[1].Designator)
	}
	if r.Ends[0].Lon >= centreLon || r.Ends[1].Lon <= centreLon {
		t.Fatalf("09L should be the west end and 27R the east: %+v", r.Ends)
	}
	if d := domain.DistanceM(r.Ends[0].Lat, r.Ends[0].Lon, r.Ends[1].Lat, r.Ends[1].Lon); math.Abs(d-3600) > 1 {
		t.Fatalf("ends are %.1f m apart; the runway is 3600 m", d)
	}
	if math.Abs(r.Ends[0].AltFt-24*metresToFeet) > 0.01 {
		t.Fatalf("09L altitude = %.1f ft, want the centre's %.1f ft", r.Ends[0].AltFt, 24*metresToFeet)
	}
}

// A stand is placed by its offset from the airport reference point.
func TestAStandIsPlacedFromTheAirportReference(t *testing.T) {
	l := buildLayout("EGLL", airportRow{Lat: 51.4775, Lon: -0.4614}, nil,
		[]parkingRow{{Name: 12, Number: 12, Radius: 30, BiasX: 300, BiasZ: -400}})
	s := l.Stands[0]
	if s.Name != "A12" || s.RadiusM != 30 {
		t.Fatalf("stand = %+v, want A12 with a 30 m radius", s)
	}
	if d := domain.DistanceM(51.4775, -0.4614, s.Lat, s.Lon); math.Abs(d-500) > 0.5 {
		t.Fatalf("stand is %.1f m from the reference; its offset is 500 m", d)
	}
	if s.Lat >= 51.4775 || s.Lon <= -0.4614 {
		t.Fatalf("stand at %.5f,%.5f is not south-east of the reference", s.Lat, s.Lon)
	}
}

func TestRunwayDesignatorsReadLikeTheChart(t *testing.T) {
	cases := []struct {
		number, designator int32
		want               string
	}{
		{9, 1, "09L"}, {27, 2, "27R"}, {18, 3, "18C"}, {36, 0, "36"},
		{4, 5, "04A"}, {37, 0, "N"}, {44, 0, "NW"}, {0, 0, ""},
	}
	for _, c := range cases {
		if got := runwayDesignator(c.number, c.designator); got != c.want {
			t.Errorf("runwayDesignator(%d, %d) = %q, want %q", c.number, c.designator, got, c.want)
		}
	}
}

func TestStandNamesReadLikeTheChart(t *testing.T) {
	cases := []struct {
		name, suffix int32
		number       uint32
		want         string
	}{
		{12, 0, 12, "A12"}, // GATE_A 12
		{37, 0, 3, "Z3"},   // GATE_Z 3
		{10, 0, 22, "22"},  // GATE 22
		{1, 0, 5, "5"},     // PARKING 5
		{2, 0, 5, "N5"},    // N_PARKING 5
		{13, 13, 4, "B4B"}, // GATE_B 4, suffix B
	}
	for _, c := range cases {
		if got := parkingName(c.name, c.suffix, c.number); got != c.want {
			t.Errorf("parkingName(%d, %d, %d) = %q, want %q", c.name, c.suffix, c.number, got, c.want)
		}
	}
}

// The airport looked up is the closest one, and none at all out at sea.
func TestTheNearestAirportIsChosen(t *testing.T) {
	airports := []airportEntry{
		{ICAO: "EGLC", Lat: 51.5053, Lon: 0.0553},
		{ICAO: "EGLL", Lat: 51.4775, Lon: -0.4614},
		{ICAO: "", Lat: 51.4700, Lon: -0.4500},
	}
	if a, ok := nearestAirport(airports, 51.4700, -0.4500, 10_000); !ok || a.ICAO != "EGLL" {
		t.Fatalf("at Heathrow chose %+v (ok=%v)", a, ok)
	}
	if _, ok := nearestAirport(airports, 50.0, -10.0, 10_000); ok {
		t.Fatal("chose an airport hundreds of kilometres away")
	}
}
