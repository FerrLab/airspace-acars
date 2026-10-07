package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

const (
	// layoutRetryEvery is the shortest gap between two layout lookups. A
	// lookup is cheap once warm but not free (a SimConnect round trip, or an
	// apt.dat seek), and while there is no airport to find, asking every
	// position tick would only find nothing faster. Thirty seconds of nulls
	// after arriving somewhere new is the cost.
	layoutRetryEvery = 30 * time.Second

	// layoutFetchTimeout bounds one lookup. X-Plane's first one waits on an
	// apt.dat index that takes seconds to build; when it gives up, the next
	// retry finds the index ready.
	layoutFetchTimeout = 10 * time.Second

	// layoutPrefetchAGLFt is the height below which the arrival airport is
	// looked up before touchdown, so the runway is known on the first report
	// after the wheels meet it instead of up to layoutRetryEvery later.
	layoutPrefetchAGLFt = 2500.0
)

// groundFix is where on the airport the aircraft is: on a runway, on a
// stand, or (both nil) neither.
type groundFix struct {
	Runway *domain.Runway
	Stand  *domain.Stand
}

// groundLocator tells position reports which runway or stand the aircraft is
// on. It keeps the current airport's layout and refreshes it on its own
// goroutine, so the sampler that asks never waits on the simulator.
type groundLocator struct {
	// provider returns the connected simulator's layout source, or nil when
	// the adapter has none. It is looked up per fetch because a reconnect
	// swaps the adapter.
	provider func() domain.AirportLayoutProvider
	now      func() time.Time

	requests chan layoutRequest
	stopCh   chan struct{}
	stopped  chan struct{}

	// mu guards layout, lastAttempt, fetching and failures.
	mu          sync.Mutex
	layout      *domain.AirportLayout
	lastAttempt time.Time
	fetching    bool
	failures    int
}

type layoutRequest struct{ lat, lon float64 }

func newGroundLocator(provider func() domain.AirportLayoutProvider) *groundLocator {
	return &groundLocator{
		provider: provider,
		now:      time.Now,
		requests: make(chan layoutRequest, 1),
		stopCh:   make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

// startGroundLocator starts a locator reading layouts from whichever
// simulator is connected.
func (a *App) startGroundLocator() *groundLocator {
	g := newGroundLocator(a.layoutProvider)
	go g.run()
	return g
}

// layoutProvider returns the connected adapter as a layout source, if it is
// one.
func (a *App) layoutProvider() domain.AirportLayoutProvider {
	a.simMu.Lock()
	connector := a.connector
	a.simMu.Unlock()
	p, _ := connector.(domain.AirportLayoutProvider)
	return p
}

// Observe places the aircraft on the airport. It never blocks: when the
// layout it needs is not in hand it asks for it and reports nothing this
// time.
func (g *groundLocator) Observe(fd *domain.FlightData) groundFix {
	if g == nil || fd == nil {
		return groundFix{}
	}
	lat, lon := fd.Position.Latitude, fd.Position.Longitude
	onGround := fd.Sensors.OnGround
	if !onGround && fd.Position.AltitudeAGL > layoutPrefetchAGLFt {
		return groundFix{}
	}

	g.mu.Lock()
	layout := g.layout
	covered := layout.Covers(lat, lon)
	ask := !covered && !g.fetching && g.now().Sub(g.lastAttempt) >= layoutRetryEvery
	if ask {
		g.fetching = true
		g.lastAttempt = g.now()
	}
	g.mu.Unlock()

	if ask {
		select {
		case g.requests <- layoutRequest{lat, lon}:
		default:
			// Unreachable while fetching guards the channel, but a full
			// channel must never stall the sampler.
		}
	}
	if !onGround || !covered {
		return groundFix{}
	}
	at := domain.GroundState{
		Lat: lat, Lon: lon,
		HeadingTrue:   fd.Attitude.HeadingTrue,
		GroundSpeedKt: fd.Attitude.GS,
	}
	return groundFix{
		Runway: domain.LocateRunway(layout, at),
		Stand:  domain.LocateStand(layout, at),
	}
}

// Stop ends the locator's goroutine.
func (g *groundLocator) Stop() {
	if g == nil {
		return
	}
	close(g.stopCh)
	<-g.stopped
}

func (g *groundLocator) run() {
	defer close(g.stopped)
	defer observability.Recover()
	for {
		select {
		case <-g.stopCh:
			return
		case req := <-g.requests:
			g.fetch(req)
		}
	}
}

// fetch looks the airport up and keeps the answer. No airport is an answer
// too: the old layout is dropped, so a report never names a runway at the
// field the aircraft left.
func (g *groundLocator) fetch(req layoutRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), layoutFetchTimeout)
	defer cancel()
	ctx, span := observability.Start(ctx, "airport.layout.fetch")
	defer span.Finish()

	var layout *domain.AirportLayout
	err := domain.ErrNoAirportData
	if p := g.provider(); p != nil {
		layout, err = p.NearestAirportLayout(ctx, req.lat, req.lon)
	}
	if err == nil && layout == nil {
		err = domain.ErrNoAirportData
	}

	g.mu.Lock()
	g.fetching = false
	g.layout = layout
	failures := g.failures
	if err == nil || errors.Is(err, domain.ErrNoAirportData) {
		g.failures = 0
	} else {
		g.failures++
	}
	g.mu.Unlock()

	switch {
	case err == nil:
		span.Set("airport.icao", layout.ICAO,
			"airport.runways", len(layout.Runways),
			"airport.stands", len(layout.Stands))
		observability.Count("airport.layout_fetched")
		slog.Debug("airport layout loaded", "airport.icao", layout.ICAO,
			"airport.runways", len(layout.Runways), "airport.stands", len(layout.Stands))
	case errors.Is(err, domain.ErrNoAirportData):
		span.Expected(err)
		slog.Debug("no airport layout here", "error", err)
	default:
		// Runway and stand are extras on the report; a simulator that will
		// not describe its airports is worth one warning per streak, then
		// silence while it keeps failing the same way.
		span.Expected(err)
		observability.Count("airport.layout_failed")
		if failures == 0 {
			slog.Warn("could not read the airport layout from the simulator", "error", err)
		} else {
			slog.Debug("airport layout lookup still failing", "error", err, "failures", failures+1)
		}
	}
}

// runwayReport is the "runway" object of a position report: both physical
// ends of the runway the aircraft is on.
type runwayReport struct {
	Ends [2]runwayEndReport `json:"ends"`
}

type runwayEndReport struct {
	Designator string      `json:"designator"`
	Latitude   measurement `json:"latitude"`
	Longitude  measurement `json:"longitude"`
	Altitude   measurement `json:"altitude"`
}

// standReport is the "stand" object of a position report.
type standReport struct {
	Name      string      `json:"name"`
	Latitude  measurement `json:"latitude"`
	Longitude measurement `json:"longitude"`
}

// newRunwayReport returns the report object for a runway, or a nil pointer,
// which encodes as the null the server reads as "not on a runway".
func newRunwayReport(r *domain.Runway) *runwayReport {
	if r == nil {
		return nil
	}
	var out runwayReport
	for i, e := range r.Ends {
		out.Ends[i] = runwayEndReport{
			Designator: e.Designator,
			Latitude:   m(e.Lat, "deg"),
			Longitude:  m(e.Lon, "deg"),
			Altitude:   m(e.AltFt, "ft"),
		}
	}
	return &out
}

// newStandReport returns the report object for a stand, or a nil pointer
// for null.
func newStandReport(s *domain.Stand) *standReport {
	if s == nil {
		return nil
	}
	return &standReport{
		Name:      s.Name,
		Latitude:  m(s.Lat, "deg"),
		Longitude: m(s.Lon, "deg"),
	}
}
