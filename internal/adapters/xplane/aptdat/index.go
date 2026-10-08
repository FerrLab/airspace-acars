package aptdat

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"airspace-acars/internal/domain"
)

// errNotAnAirport means the bytes at an indexed offset do not start an
// airport: the file changed under the index.
var errNotAnAirport = errors.New("no airport at the indexed offset")

const (
	// maxAirportBytes bounds the read of one airport's rows. The largest
	// airports in the global apt.dat run to a few megabytes with their taxi
	// network and pavement outlines; anything past this is not an airport.
	maxAirportBytes = 32 << 20

	// coverMarginDeg widens an airport's runway-and-stand envelope when
	// deciding whether the aircraft is at it, matching the margin
	// AirportLayout.Covers gives a loaded layout (1.5 km).
	coverMarginDeg = 1500.0 / 111_320.0

	// maxAirportDistanceM is how far an airport's reference point may be,
	// when no airport's envelope contains the aircraft, for it to count as
	// the airport the aircraft is at.
	maxAirportDistanceM = 10_000.0
)

// indexEntry locates one airport in one apt.dat and summarises where it is,
// so a lookup reads only the airport it needs out of a file of hundreds of
// megabytes.
type indexEntry struct {
	ident  string
	file   string
	offset int64
	length int64

	refLat, refLon float64
	minLat, minLon float64
	maxLat, maxLon float64
	hasBounds      bool
}

type index struct {
	root    string
	entries []indexEntry
	files   int
}

// buildIndex scans the apt.dat files in priority order. The first file to
// define an airport wins, as it does in the simulator, so a custom scenery
// pack replaces the global version of the same airport.
func buildIndex(root string, files []string) (*index, error) {
	idx := &index{root: root}
	seen := map[string]bool{}
	for _, file := range files {
		n, err := idx.scanFile(file, seen)
		if err != nil {
			// One unreadable pack must not cost the rest; the global
			// airports alone cover nearly every field.
			continue
		}
		if n > 0 {
			idx.files++
		}
	}
	if len(idx.entries) == 0 {
		return nil, fmt.Errorf("%w: no airports in %d apt.dat files", domain.ErrNoAirportData, len(files))
	}
	return idx, nil
}

// scanFile adds the land airports of one apt.dat to the index. It reads the
// file once, line by line, keeping each airport's byte range and the extent
// of its runways and stands.
func (idx *index) scanFile(file string, seen map[string]bool) (int, error) {
	f, err := os.Open(file)
	if err != nil {
		return 0, fmt.Errorf("open apt.dat: %w", err)
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 1<<20)
	var (
		offset int64
		cur    *indexEntry
		rec    airportRecord
		added  int
	)
	flush := func(end int64) {
		if cur == nil {
			return
		}
		cur.length = end - cur.offset
		if !seen[cur.ident] && cur.length <= maxAirportBytes {
			cur.refLat, cur.refLon = rec.reference()
			if cur.refLat != 0 || cur.refLon != 0 {
				seen[cur.ident] = true
				idx.entries = append(idx.entries, *cur)
				added++
			}
		}
		cur = nil
	}
	for {
		line, err := r.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			// A line longer than the buffer is no row we read; skip the
			// rest of it.
			n := int64(len(line))
			for errors.Is(err, bufio.ErrBufferFull) {
				line, err = r.ReadSlice('\n')
				n += int64(len(line))
			}
			offset += n
			continue
		}
		start := offset
		offset += int64(len(line))

		code := string(rowCode(line))
		switch {
		case isAirportHeader(code):
			flush(start)
			if code == rowLandAirport {
				fields := strings.Fields(string(line))
				if len(fields) >= 5 {
					cur = &indexEntry{ident: fields[4], file: file, offset: start}
					rec = airportRecord{}
				}
			}
		case cur != nil && (code == rowRunway || code == rowStand || code == rowStartup || code == rowMetadata):
			before := len(rec.runways) + len(rec.stands)
			rec.addRow(strings.Fields(string(line)))
			if len(rec.runways)+len(rec.stands) > before {
				cur.grow(&rec)
			}
		}

		if err == io.EOF {
			flush(offset)
			return added, nil
		}
		if err != nil {
			return added, fmt.Errorf("read apt.dat: %w", err)
		}
	}
}

// grow extends the entry's envelope by the runway or stand just added.
func (e *indexEntry) grow(rec *airportRecord) {
	add := func(lat, lon float64) {
		if !e.hasBounds {
			e.minLat, e.maxLat, e.minLon, e.maxLon = lat, lat, lon, lon
			e.hasBounds = true
			return
		}
		e.minLat, e.maxLat = math.Min(e.minLat, lat), math.Max(e.maxLat, lat)
		e.minLon, e.maxLon = math.Min(e.minLon, lon), math.Max(e.maxLon, lon)
	}
	if n := len(rec.stands); n > 0 {
		s := rec.stands[n-1]
		add(s.Lat, s.Lon)
	}
	if n := len(rec.runways); n > 0 {
		for _, end := range rec.runways[n-1].Ends {
			add(end.Lat, end.Lon)
		}
	}
}

// covers reports whether the position is inside the airport's envelope.
func (e *indexEntry) covers(lat, lon float64) bool {
	if !e.hasBounds {
		return false
	}
	lonMargin := coverMarginDeg / math.Max(0.01, math.Cos(lat*math.Pi/180))
	return lat >= e.minLat-coverMarginDeg && lat <= e.maxLat+coverMarginDeg &&
		lon >= e.minLon-lonMargin && lon <= e.maxLon+lonMargin
}

// pick chooses the airport the aircraft is at. An airport whose runways and
// stands surround it comes first, nearest reference point among those; a big
// airport's reference can be farther from a remote stand than a small
// neighbour's is. Failing that, the nearest reference within reach.
func (idx *index) pick(lat, lon float64) (indexEntry, bool) {
	var best indexEntry
	bestM, bestCovers := math.Inf(1), false
	for _, e := range idx.entries {
		covers := e.covers(lat, lon)
		if bestCovers && !covers {
			continue
		}
		d := domain.DistanceM(lat, lon, e.refLat, e.refLon)
		if (covers && !bestCovers) || d < bestM {
			best, bestM, bestCovers = e, d, covers
		}
	}
	return best, bestCovers || bestM <= maxAirportDistanceM
}

// readAirport reads the indexed airport's rows back out of its file.
func readAirport(e indexEntry) (*airportRecord, error) {
	f, err := os.Open(e.file)
	if err != nil {
		return nil, fmt.Errorf("open apt.dat: %w", err)
	}
	defer f.Close()
	rec, err := parseAirport(io.NewSectionReader(f, e.offset, e.length))
	if err != nil {
		return nil, fmt.Errorf("read airport %s: %w", e.ident, err)
	}
	if rec.ident != e.ident {
		return nil, fmt.Errorf("read airport %s: %w", e.ident, errNotAnAirport)
	}
	return rec, nil
}

// sceneryFiles lists the apt.dat files under an X-Plane root in the order
// the simulator gives them priority: the enabled custom scenery packs as
// scenery_packs.ini orders them, with the global airports where the
// *GLOBAL_AIRPORTS* line puts them (X-Plane 12), or last (X-Plane 11).
func sceneryFiles(root string) []string {
	global := ""
	for _, rel := range []string{
		"Global Scenery/Global Airports/Earth nav data/apt.dat",            // X-Plane 12
		"Resources/default scenery/default apt dat/Earth nav data/apt.dat", // X-Plane 11
	} {
		if p := filepath.Join(root, filepath.FromSlash(rel)); fileExists(p) {
			global = p
			break
		}
	}

	var files []string
	placedGlobal := false
	if ini, err := os.ReadFile(filepath.Join(root, "Custom Scenery", "scenery_packs.ini")); err == nil {
		for _, raw := range bytes.Split(ini, []byte("\n")) {
			line := strings.TrimSpace(string(raw))
			pack, ok := strings.CutPrefix(line, "SCENERY_PACK ")
			if !ok {
				// SCENERY_PACK_DISABLED and the header lines.
				continue
			}
			pack = strings.TrimSpace(pack)
			if pack == "*GLOBAL_AIRPORTS*" {
				if global != "" && !placedGlobal {
					files = append(files, global)
					placedGlobal = true
				}
				continue
			}
			dir := filepath.FromSlash(pack)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(root, dir)
			}
			if p := filepath.Join(dir, "Earth nav data", "apt.dat"); fileExists(p) {
				files = append(files, p)
			}
		}
	}
	if global != "" && !placedGlobal {
		files = append(files, global)
	}
	return files
}

// cifpElevations returns the runway end elevations of an airport from the
// CIFP navdata: the pilot's AIRAC cycle in Custom Data when installed, the
// data X-Plane ships otherwise. Missing navdata is no error; the ends keep
// the airport elevation.
func cifpElevations(root, icao string) map[string]float64 {
	if !validIdent(icao) {
		return nil
	}
	for _, dir := range []string{
		filepath.Join(root, "Custom Data", "CIFP"),
		filepath.Join(root, "Resources", "default data", "CIFP"),
	} {
		f, err := os.Open(filepath.Join(dir, icao+".dat"))
		if err != nil {
			continue
		}
		elev := parseCIFPElevations(io.LimitReader(f, maxAirportBytes))
		f.Close()
		return elev
	}
	return nil
}

// validIdent guards the CIFP file name: an airport ident is a few letters
// and digits, never a path.
func validIdent(s string) bool {
	if len(s) < 2 || len(s) > 7 {
		return false
	}
	for _, c := range s {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// applyElevations gives each runway end its own elevation where the navdata
// has one.
func applyElevations(l *domain.AirportLayout, elev map[string]float64) {
	for i := range l.Runways {
		for j := range l.Runways[i].Ends {
			end := &l.Runways[i].Ends[j]
			if ft, ok := elev[end.Designator]; ok {
				end.AltFt = ft
			}
		}
	}
}
