// Package aptdat reads airport layouts from X-Plane's own scenery: the
// apt.dat files of its global airports and custom scenery packs, and the
// CIFP navdata that gives each runway end an elevation.
//
// X-Plane's UDP interface carries datarefs, not airport data, so this is the
// only way to know which runway or stand the aircraft is on. It works when
// the ACARS runs on the same machine as X-Plane, or when the pilot points the
// xplanePath setting at a shared install folder.
package aptdat

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"strings"

	"airspace-acars/internal/domain"
)

// Row codes of the apt.dat format (XP-APT1100 and XP-APT1200 specs).
const (
	rowLandAirport = "1"
	rowSeaplane    = "16"
	rowHeliport    = "17"
	rowEnd         = "99"
	rowRunway      = "100"
	rowStartup     = "15"   // legacy start location, before 1300 existed
	rowStand       = "1300" // startup location: gate, ramp, tie-down, hangar
	rowMetadata    = "1302"
)

// isAirportHeader reports whether a row starts a new airport (or ends the
// file), which is where the current airport's rows stop.
func isAirportHeader(code string) bool {
	return code == rowLandAirport || code == rowSeaplane || code == rowHeliport || code == rowEnd
}

// rowCode is the first field of an apt.dat line, without allocating.
func rowCode(line []byte) []byte {
	line = bytes.TrimLeft(line, " \t")
	if i := bytes.IndexAny(line, " \t\r\n"); i >= 0 {
		return line[:i]
	}
	return line
}

// airportRecord is what one airport's rows say about it.
type airportRecord struct {
	ident       string // the apt.dat ID; often, but not always, the ICAO
	icao        string // 1302 icao_code, when given
	elevationFt float64
	datumLat    float64
	datumLon    float64
	hasDatum    bool
	runways     []domain.Runway
	stands      []domain.Stand
}

// parseAirport reads the rows of one airport, from its header to the next.
// A row it cannot read is skipped rather than failing the airport: one
// malformed stand in a custom scenery pack must not cost the runway.
func parseAirport(r io.Reader) (*airportRecord, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var rec *airportRecord
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		if isAirportHeader(f[0]) {
			if rec != nil {
				break
			}
			if f[0] == rowEnd || len(f) < 5 {
				return nil, errNotAnAirport
			}
			rec = &airportRecord{ident: f[4]}
			rec.elevationFt, _ = strconv.ParseFloat(f[1], 64)
			continue
		}
		if rec == nil {
			continue
		}
		rec.addRow(f)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, errNotAnAirport
	}
	return rec, nil
}

func (rec *airportRecord) addRow(f []string) {
	switch f[0] {
	case rowRunway:
		if rw, ok := parseRunway(f, rec.elevationFt); ok {
			rec.runways = append(rec.runways, rw)
		}
	case rowStand:
		// 1300 lat lon heading type airplane-types name...
		if len(f) >= 7 {
			rec.addStand(f[1], f[2], strings.Join(f[6:], " "))
		}
	case rowStartup:
		// 15 lat lon heading name...
		if len(f) >= 5 {
			rec.addStand(f[1], f[2], strings.Join(f[4:], " "))
		}
	case rowMetadata:
		if len(f) < 3 {
			return
		}
		switch f[1] {
		case "icao_code":
			rec.icao = f[2]
		case "datum_lat":
			rec.datumLat, _ = strconv.ParseFloat(f[2], 64)
			rec.hasDatum = true
		case "datum_lon":
			rec.datumLon, _ = strconv.ParseFloat(f[2], 64)
		}
	}
}

// standRadiusM is the size given to every X-Plane stand. apt.dat records a
// stand as a point with no size, unlike MSFS; 20 m is roughly half the span
// of a narrow-body's parking box, so an aircraft parked a little off its
// mark still counts.
const standRadiusM = 20.0

func (rec *airportRecord) addStand(lat, lon, name string) {
	la, err1 := strconv.ParseFloat(lat, 64)
	lo, err2 := strconv.ParseFloat(lon, 64)
	if err1 != nil || err2 != nil || !validLatLon(la, lo) {
		return
	}
	for _, s := range rec.stands {
		if s.Name == name {
			// Old files list a stand as both 15 and 1300.
			return
		}
	}
	rec.stands = append(rec.stands, domain.Stand{Name: name, Lat: la, Lon: lo, RadiusM: standRadiusM})
}

// parseRunway reads a 100 row: width and surface details, then eight
// fields per end starting with its number, latitude and longitude. The
// latitude and longitude are the physical end of the runway, before any
// displaced threshold.
func parseRunway(f []string, elevationFt float64) (domain.Runway, bool) {
	if len(f) < 20 {
		return domain.Runway{}, false
	}
	width, err := strconv.ParseFloat(f[1], 64)
	if err != nil {
		return domain.Runway{}, false
	}
	var rw domain.Runway
	rw.WidthM = width
	for i, at := range []int{8, 17} {
		lat, err1 := strconv.ParseFloat(f[at+1], 64)
		lon, err2 := strconv.ParseFloat(f[at+2], 64)
		if err1 != nil || err2 != nil || !validLatLon(lat, lon) {
			return domain.Runway{}, false
		}
		rw.Ends[i] = domain.RunwayEnd{Designator: f[at], Lat: lat, Lon: lon, AltFt: elevationFt}
	}
	return rw, true
}

func validLatLon(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

// layout turns the record into the domain shape. Its reference point is the
// datum when the file has one, otherwise the middle of its runways, then of
// its stands.
func (rec *airportRecord) layout() *domain.AirportLayout {
	icao := rec.icao
	if icao == "" {
		icao = rec.ident
	}
	l := &domain.AirportLayout{
		ICAO: icao, ElevationFt: rec.elevationFt,
		Runways: rec.runways, Stands: rec.stands,
	}
	l.RefLat, l.RefLon = rec.reference()
	return l
}

func (rec *airportRecord) reference() (float64, float64) {
	if rec.hasDatum {
		return rec.datumLat, rec.datumLon
	}
	var lat, lon float64
	n := 0
	for _, r := range rec.runways {
		for _, e := range r.Ends {
			lat, lon, n = lat+e.Lat, lon+e.Lon, n+1
		}
	}
	if n == 0 {
		for _, s := range rec.stands {
			lat, lon, n = lat+s.Lat, lon+s.Lon, n+1
		}
	}
	if n == 0 {
		return 0, 0
	}
	return lat / float64(n), lon / float64(n)
}

// parseCIFPElevations reads the RWY records of an airport's CIFP file and
// returns each runway end's threshold elevation in feet, by designator.
//
// A record is "RWY:RW09L,<gradient>,<ellipsoid height>,<threshold
// elevation>,..." followed by ";" and its position. The elevation is that of
// the landing threshold, which for a displaced threshold is a few hundred
// metres in from the physical end; on any real runway the two differ by a
// foot or two, and it is the only per-end elevation X-Plane publishes.
func parseCIFPElevations(r io.Reader) map[string]float64 {
	out := map[string]float64{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "RWY:") {
			continue
		}
		head, _, _ := strings.Cut(line[len("RWY:"):], ";")
		f := strings.Split(head, ",")
		if len(f) < 4 {
			continue
		}
		id := strings.TrimSpace(f[0])
		if !strings.HasPrefix(id, "RW") || len(id) < 4 {
			continue
		}
		elev, err := strconv.ParseFloat(strings.TrimSpace(f[3]), 64)
		if err != nil {
			continue
		}
		out[id[2:]] = elev
	}
	return out
}
