package domain

import (
	"math"
	"testing"
)

// egll09L27R is Heathrow's northern runway: 3902 m long, 50 m wide, running
// almost exactly east–west.
var egll09L27R = Runway{
	Ends: [2]RunwayEnd{
		{Designator: "09L", Lat: 51.477500, Lon: -0.484961},
		{Designator: "27R", Lat: 51.477661, Lon: -0.433128},
	},
	WidthM: 50,
}

// crossing is a made-up north–south runway that crosses 09L/27R at its
// middle, for the intersection cases.
var crossing = Runway{
	Ends: [2]RunwayEnd{
		{Designator: "18", Lat: 51.4900, Lon: -0.4590},
		{Designator: "36", Lat: 51.4650, Lon: -0.4590},
	},
	WidthM: 45,
}

func heathrow() *AirportLayout {
	return &AirportLayout{
		ICAO: "EGLL", RefLat: 51.4775, RefLon: -0.4614,
		Runways: []Runway{egll09L27R, crossing},
	}
}

// A runway is reported only while the aircraft is on its pavement, and at an
// intersection it is the one the aircraft is lined up with.
func TestTheRunwayIsTheOneUnderTheAircraft(t *testing.T) {
	l := heathrow()
	// Points along 09L/27R, offset north (positive) or south of the
	// centreline by the given metres.
	along := func(frac, offsetNorthM float64) (float64, float64) {
		a, b := egll09L27R.Ends[0], egll09L27R.Ends[1]
		lat := a.Lat + (b.Lat-a.Lat)*frac
		lon := a.Lon + (b.Lon-a.Lon)*frac
		return OffsetMeters(lat, lon, 0, offsetNorthM)
	}

	cases := []struct {
		name    string
		frac    float64
		offset  float64
		heading float64
		want    string // first-end designator, or "" for none
	}{
		{"lined up on the centreline", 0.2, 0, 90, "09L"},
		{"on the pavement near the edge", 0.2, 22, 90, "09L"},
		{"on the grass beside it", 0.2, 40, 90, ""},
		{"short of the 09L end", -0.01, 0, 90, ""},
		{"beyond the 27R end", 1.01, 0, 270, ""},
		{"rolling west through the intersection", 0.5, 0, 270, "09L"},
		{"crossing it on the north-south runway", 0.5, 0, 180, "18"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lat, lon := along(c.frac, c.offset)
			if c.name == "crossing it on the north-south runway" {
				lat, lon = OffsetMeters(lat, -0.4590, 0, 0)
			}
			got := LocateRunway(l, GroundState{Lat: lat, Lon: lon, HeadingTrue: c.heading})
			switch {
			case c.want == "" && got != nil:
				t.Fatalf("reported runway %s; the aircraft is not on a runway", got.Ends[0].Designator)
			case c.want != "" && got == nil:
				t.Fatalf("reported no runway; the aircraft is on %s", c.want)
			case c.want != "" && got.Ends[0].Designator != c.want:
				t.Fatalf("reported runway %s; the aircraft is on %s", got.Ends[0].Designator, c.want)
			}
		})
	}
}

// A runway across the antimeridian is still a runway: the projection must
// not see its two ends as 360 degrees apart.
func TestARunwayAcrossTheAntimeridianIsFound(t *testing.T) {
	l := &AirportLayout{RefLat: -16.0, RefLon: 180, Runways: []Runway{{
		Ends: [2]RunwayEnd{
			{Designator: "09", Lat: -16.0, Lon: 179.99},
			{Designator: "27", Lat: -16.0, Lon: -179.99},
		},
		WidthM: 45,
	}}}
	if LocateRunway(l, GroundState{Lat: -16.0, Lon: 180.0, HeadingTrue: 90}) == nil {
		t.Fatal("the aircraft on the antimeridian is not on the runway that spans it")
	}
}

// A runway whose scenery gives it no width still has one.
func TestARunwayWithoutAWidthIsGivenOne(t *testing.T) {
	r := egll09L27R
	r.WidthM = 0
	l := &AirportLayout{Runways: []Runway{r}}
	a := r.Ends[0]
	lat, lon := OffsetMeters(a.Lat, a.Lon+0.01, 0, 10)
	if LocateRunway(l, GroundState{Lat: lat, Lon: lon, HeadingTrue: 90}) == nil {
		t.Fatal("10 m off the centreline of a widthless runway is reported as off it")
	}
}

func TestNoLayoutLocatesNothing(t *testing.T) {
	if LocateRunway(nil, GroundState{}) != nil || LocateStand(nil, GroundState{}) != nil {
		t.Fatal("located something on an airport that does not exist")
	}
}

// The layout stays in use while the aircraft is anywhere on the field, and is
// dropped once it has left.
func TestALayoutCoversItsAirfield(t *testing.T) {
	l := heathrow()
	if !l.Covers(51.4700, -0.4500) {
		t.Fatal("a point between the runways is reported as away from the airport")
	}
	lat, lon := Destination(51.4775, -0.4614, 0, 10_000)
	if l.Covers(lat, lon) {
		t.Fatal("a point 10 km north is reported as at the airport")
	}
	var none *AirportLayout
	if none.Covers(51.47, -0.46) {
		t.Fatal("no layout covers everything")
	}
}

func TestGeometryHelpersAgree(t *testing.T) {
	lat, lon := Destination(51.4775, -0.4614, 90, 1000)
	if d := DistanceM(51.4775, -0.4614, lat, lon); math.Abs(d-1000) > 0.5 {
		t.Fatalf("Destination 1000 m east lands %.2f m away", d)
	}
	lat, lon = OffsetMeters(51.4775, -0.4614, 300, 400)
	if d := DistanceM(51.4775, -0.4614, lat, lon); math.Abs(d-500) > 0.5 {
		t.Fatalf("a 300 m east, 400 m north offset lands %.2f m away, not 500", d)
	}
}

// Two stands side by side, 40 m apart, the way contact gates sit.
func gates() *AirportLayout {
	a12Lat, a12Lon := 51.47120, -0.45980
	a13Lat, a13Lon := OffsetMeters(a12Lat, a12Lon, 40, 0)
	return &AirportLayout{Stands: []Stand{
		{Name: "A12", Lat: a12Lat, Lon: a12Lon, RadiusM: 25},
		{Name: "A13", Lat: a13Lat, Lon: a13Lon, RadiusM: 25},
	}}
}

// The stand is the one the aircraft is parked on: nearest when stands
// overlap, none once it is well clear of every stand.
func TestTheStandIsTheOneTheAircraftIsParkedOn(t *testing.T) {
	l := gates()
	a12 := l.Stands[0]
	at := func(east, north float64) GroundState {
		lat, lon := OffsetMeters(a12.Lat, a12.Lon, east, north)
		return GroundState{Lat: lat, Lon: lon, HeadingTrue: 0}
	}
	cases := []struct {
		name string
		g    GroundState
		want string
	}{
		{"parked on the mark", at(0, 0), "A12"},
		{"parked a few metres off the mark", at(3, -4), "A12"},
		{"closer to the neighbouring stand", at(28, 0), "A13"},
		{"out on the apron", at(0, 150), ""},
		{"taxiing past at 12 kt", moving(at(0, 0), 12), ""},
		{"being pushed back at 2 kt", moving(at(0, -10), 2), "A12"},
		{"right at the 3 kt limit", moving(at(0, 0), 3), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := LocateStand(l, c.g)
			switch {
			case c.want == "" && got != nil:
				t.Fatalf("reported stand %s; the aircraft is on none", got.Name)
			case c.want != "" && (got == nil || got.Name != c.want):
				t.Fatalf("reported %v; the aircraft is on %s", got, c.want)
			}
		})
	}
}

func moving(g GroundState, kt float64) GroundState {
	g.GroundSpeedKt = kt
	return g
}

// A stand whose scenery gives no size still has one.
func TestAStandWithoutARadiusIsGivenOne(t *testing.T) {
	l := &AirportLayout{Stands: []Stand{{Name: "5", Lat: 51.47, Lon: -0.45}}}
	lat, lon := OffsetMeters(51.47, -0.45, 5, 5)
	if s := LocateStand(l, GroundState{Lat: lat, Lon: lon}); s == nil || s.Name != "5" {
		t.Fatalf("7 m from a sizeless stand reported %v, want stand 5", s)
	}
}
