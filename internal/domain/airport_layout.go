package domain

import (
	"context"
	"errors"
	"math"
)

// ErrNoAirportData means the simulator could not describe an airport near
// the aircraft: none is close enough, the scenery is not readable from here
// (X-Plane on another machine), or the lookup is still warming up. It is part
// of normal operation, and a position report carries a null runway and stand.
var ErrNoAirportData = errors.New("no airport layout available")

// AirportLayoutProvider is implemented by a simulator adapter that can read
// the layout of the airport nearest a position from the simulator's own
// scenery. It is optional: the app type-asserts the connector against it, so
// an adapter without it simply reports no runway and no stand.
type AirportLayoutProvider interface {
	// NearestAirportLayout returns the airport the position is at, or nearest
	// to, or ErrNoAirportData when there is none worth describing.
	NearestAirportLayout(ctx context.Context, lat, lon float64) (*AirportLayout, error)
}

// AirportLayout is the part of an airport that position reports care about:
// where its runways and stands are.
type AirportLayout struct {
	ICAO        string
	RefLat      float64
	RefLon      float64
	ElevationFt float64
	Runways     []Runway
	Stands      []Stand
}

// Runway is a runway as its two physical ends: where the pavement begins at
// either side, not a displaced landing threshold.
type Runway struct {
	Ends   [2]RunwayEnd
	WidthM float64
}

// RunwayEnd is one end of a runway, named by its designator ("09L").
type RunwayEnd struct {
	Designator string
	Lat        float64
	Lon        float64
	AltFt      float64
}

// Stand is a parking position: a gate, a ramp spot, a tie-down.
type Stand struct {
	Name    string
	Lat     float64
	Lon     float64
	RadiusM float64
}

// GroundState is what locating the aircraft on an airport needs to know
// about it.
type GroundState struct {
	Lat           float64
	Lon           float64
	HeadingTrue   float64
	GroundSpeedKt float64
}

const (
	// earthRadiusM is the mean Earth radius. Every distance here is a few
	// kilometres at most, where the spherical error is centimetres.
	earthRadiusM = 6371008.8

	// fallbackRunwayWidthM stands in for a runway whose scenery gives no
	// width. 45 m is the common width of an airliner runway, so a narrower
	// strip is over-matched by a few metres rather than missed altogether.
	fallbackRunwayWidthM = 45.0

	// layoutCoverMarginM is how far beyond its outermost runway end or stand
	// an aircraft still counts as at the airport. It covers aprons, hangars
	// and taxiways outside the runway envelope, so the layout is not
	// refetched while taxiing around the edge of the field.
	layoutCoverMarginM = 1500.0

	// maxStandSpeedKt is the ground speed below which an aircraft inside a
	// stand counts as on it. Taxi speed is 10-20 kt and a pushback about 2,
	// so 3 kt takes in a pushback and the last creep onto the mark, and
	// leaves out an aircraft taxiing past the gates.
	maxStandSpeedKt = 3.0

	// fallbackStandRadiusM sizes a stand whose scenery gives it no size,
	// matching what X-Plane stands get (aptdat.standRadiusM).
	fallbackStandRadiusM = 20.0
)

// Destination returns the point distM metres from (lat, lon) along the
// initial true bearing bearingDeg, on a sphere.
func Destination(lat, lon, bearingDeg, distM float64) (float64, float64) {
	φ1 := lat * math.Pi / 180
	λ1 := lon * math.Pi / 180
	θ := bearingDeg * math.Pi / 180
	δ := distM / earthRadiusM

	φ2 := math.Asin(math.Sin(φ1)*math.Cos(δ) + math.Cos(φ1)*math.Sin(δ)*math.Cos(θ))
	λ2 := λ1 + math.Atan2(math.Sin(θ)*math.Sin(δ)*math.Cos(φ1), math.Cos(δ)-math.Sin(φ1)*math.Sin(φ2))
	return φ2 * 180 / math.Pi, normalizeLon(λ2 * 180 / math.Pi)
}

// OffsetMeters returns the point east metres east and north metres north of
// (lat, lon). It is a flat-earth step, exact enough for the few hundred
// metres between an airport's reference point and its stands.
func OffsetMeters(lat, lon, east, north float64) (float64, float64) {
	dLat := north / earthRadiusM * 180 / math.Pi
	dLon := east / (earthRadiusM * math.Cos(lat*math.Pi/180)) * 180 / math.Pi
	return lat + dLat, normalizeLon(lon + dLon)
}

// DistanceM is the great-circle distance in metres between two points.
func DistanceM(lat1, lon1, lat2, lon2 float64) float64 {
	φ1, φ2 := lat1*math.Pi/180, lat2*math.Pi/180
	dφ := φ2 - φ1
	dλ := normalizeLon(lon2-lon1) * math.Pi / 180
	h := math.Sin(dφ/2)*math.Sin(dφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(dλ/2)*math.Sin(dλ/2)
	return 2 * earthRadiusM * math.Asin(math.Min(1, math.Sqrt(h)))
}

// localXY projects (lat, lon) to metres east and north of the origin. Within
// an airport this is accurate to well under a metre, and taking the longitude
// difference through normalizeLon keeps it right across the antimeridian.
func localXY(originLat, originLon, lat, lon float64) (x, y float64) {
	x = normalizeLon(lon-originLon) * math.Pi / 180 * earthRadiusM * math.Cos(originLat*math.Pi/180)
	y = (lat - originLat) * math.Pi / 180 * earthRadiusM
	return x, y
}

func normalizeLon(lon float64) float64 {
	lon = math.Mod(lon+180, 360)
	if lon < 0 {
		lon += 360
	}
	return lon - 180
}

// Covers reports whether the position is at this airport: within
// layoutCoverMarginM of the envelope of its runway ends and stands, or of its
// reference point when it has neither.
func (l *AirportLayout) Covers(lat, lon float64) bool {
	if l == nil {
		return false
	}
	minX, minY, maxX, maxY := 0.0, 0.0, 0.0, 0.0
	grow := func(pLat, pLon float64) {
		x, y := localXY(l.RefLat, l.RefLon, pLat, pLon)
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
	}
	for _, r := range l.Runways {
		grow(r.Ends[0].Lat, r.Ends[0].Lon)
		grow(r.Ends[1].Lat, r.Ends[1].Lon)
	}
	for _, s := range l.Stands {
		grow(s.Lat, s.Lon)
	}
	x, y := localXY(l.RefLat, l.RefLon, lat, lon)
	return x >= minX-layoutCoverMarginM && x <= maxX+layoutCoverMarginM &&
		y >= minY-layoutCoverMarginM && y <= maxY+layoutCoverMarginM
}

// LocateRunway returns the runway whose paved rectangle (end to end, half
// its width either side of the centreline) contains the aircraft, or nil.
//
// Where runways cross, the aircraft stands on both. It is reported on the one
// it is lined up with, so a take-off roll through an intersection does not
// flicker onto the crossing runway.
func LocateRunway(l *AirportLayout, g GroundState) *Runway {
	if l == nil {
		return nil
	}
	var best *Runway
	bestMisalign := math.Inf(1)
	for i := range l.Runways {
		r := &l.Runways[i]
		a, b := r.Ends[0], r.Ends[1]
		bx, by := localXY(a.Lat, a.Lon, b.Lat, b.Lon)
		px, py := localXY(a.Lat, a.Lon, g.Lat, g.Lon)
		length := math.Hypot(bx, by)
		if length == 0 {
			continue
		}
		ux, uy := bx/length, by/length
		along := px*ux + py*uy
		across := math.Abs(px*uy - py*ux)
		halfWidth := r.WidthM / 2
		if r.WidthM <= 0 {
			halfWidth = fallbackRunwayWidthM / 2
		}
		if along < 0 || along > length || across > halfWidth {
			continue
		}
		axis := math.Atan2(bx, by) * 180 / math.Pi
		misalign := axisDifference(g.HeadingTrue, axis)
		if misalign < bestMisalign {
			best, bestMisalign = r, misalign
		}
	}
	return best
}

// axisDifference is the angle in degrees between a heading and a runway axis,
// which has no direction: lined up either way is 0, across it is 90.
func axisDifference(heading, axis float64) float64 {
	d := math.Mod(math.Abs(heading-axis), 180)
	return math.Min(d, 180-d)
}

// LocateStand returns the stand the aircraft is parked on, or nil: the
// nearest stand whose radius contains it, provided it is all but stopped.
// The speed limit keeps an aircraft taxiing past a row of gates from being
// reported on each in turn.
func LocateStand(l *AirportLayout, g GroundState) *Stand {
	if l == nil || g.GroundSpeedKt >= maxStandSpeedKt {
		return nil
	}
	var best *Stand
	bestM := math.Inf(1)
	for i := range l.Stands {
		s := &l.Stands[i]
		radius := s.RadiusM
		if radius <= 0 {
			radius = fallbackStandRadiusM
		}
		if d := DistanceM(g.Lat, g.Lon, s.Lat, s.Lon); d <= radius && d < bestM {
			best, bestM = s, d
		}
	}
	return best
}
