package aptdat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

// Source answers airport layout lookups from an X-Plane installation. The
// index of its apt.dat files is built once, in the background, on the first
// lookup, and rebuilt only if the installation it reads from changes.
type Source struct {
	// rootOverride returns the xplanePath setting; empty means detect the
	// installation from the running simulator.
	rootOverride func() string
	// findRunning returns the folder of the running X-Plane, if any.
	findRunning func() (string, bool)

	// mu guards current.
	mu      sync.Mutex
	current *indexBuild
}

// indexBuild is the index of one X-Plane root, finished once done closes.
type indexBuild struct {
	root string
	done chan struct{}
	idx  *index
	err  error
}

// NewSource returns a Source that reads the installation named by
// rootOverride, or the one X-Plane is running from when that is empty.
func NewSource(rootOverride func() string) *Source {
	return &Source{rootOverride: rootOverride, findRunning: runningXPlaneRoot}
}

// NearestAirportLayout implements domain.AirportLayoutProvider.
func (s *Source) NearestAirportLayout(ctx context.Context, lat, lon float64) (*domain.AirportLayout, error) {
	root := s.root()
	if root == "" {
		return nil, fmt.Errorf("%w: X-Plane folder not found", domain.ErrNoAirportData)
	}
	b := s.build(root)
	select {
	case <-b.done:
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: apt.dat index still building", domain.ErrNoAirportData)
	}
	if b.err != nil {
		return nil, b.err
	}

	entry, ok := b.idx.pick(lat, lon)
	if !ok {
		return nil, domain.ErrNoAirportData
	}
	rec, err := readAirport(entry)
	if err != nil {
		return nil, err
	}
	l := rec.layout()
	applyElevations(l, cifpElevations(root, l.ICAO))
	return l, nil
}

func (s *Source) root() string {
	if s.rootOverride != nil {
		if r := strings.TrimSpace(s.rootOverride()); r != "" {
			return r
		}
	}
	if s.findRunning != nil {
		if r, ok := s.findRunning(); ok {
			return r
		}
	}
	return ""
}

// build returns the index of root, starting it if it is not already built
// or building.
func (s *Source) build(root string) *indexBuild {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil && s.current.root == root {
		return s.current
	}
	b := &indexBuild{root: root, done: make(chan struct{})}
	s.current = b
	go b.run()
	return b
}

func (b *indexBuild) run() {
	defer close(b.done)
	defer observability.Recover()
	_, span := observability.Start(context.Background(), "aptdat.index.build")
	defer span.Finish()

	files := sceneryFiles(b.root)
	b.idx, b.err = buildIndex(b.root, files)
	if b.err != nil {
		span.Expected(b.err)
		// No path in the message: it is under the pilot's profile as often
		// as not.
		slog.Warn("no X-Plane airports found; runway and stand will not be reported",
			"aptdat.files", len(files))
		return
	}
	span.Set("aptdat.files", b.idx.files, "aptdat.airports", len(b.idx.entries))
	slog.Info("X-Plane airport index built",
		"aptdat.files", b.idx.files, "aptdat.airports", len(b.idx.entries))
}
