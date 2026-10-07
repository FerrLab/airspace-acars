//go:build windows

package simconnect

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"unsafe"

	"airspace-acars/internal/domain"
	sim "airspace-acars/internal/simconnect"
)

const (
	// facilityRequestBase starts the request IDs of airport lookups, far
	// above the profile data requests (requestIDBase) so a reply is never
	// mistaken for the other kind.
	facilityRequestBase = 1 << 20

	// maxAirportDistanceM is how far the nearest airport's reference point
	// may be for the aircraft to count as at it. The largest airports spread
	// a few kilometres from their reference; ten keeps their remote stands
	// and leaves out the airport across the bay.
	maxAirportDistanceM = 10_000.0

	// facilityLookupTimeout abandons a lookup whose reply never completes.
	// A list request into an empty facility cache (mid-ocean) may never be
	// answered at all, so silence there is read as "no airport".
	facilityLookupTimeout = 8 * time.Second
)

// errFacilityLookupBusy is returned when a lookup is asked for while
// another is in flight. The caller retries later.
var errFacilityLookupBusy = errors.New("airport lookup already in flight")

// layoutQuery is a request, from any goroutine, for the dispatch loop to
// look up the airport at a position.
type layoutQuery struct {
	lat, lon float64
	reply    chan layoutResult
}

type layoutResult struct {
	layout *domain.AirportLayout
	err    error
}

// NearestAirportLayout implements domain.AirportLayoutProvider. SimConnect
// calls must come from the thread that owns the session, so the lookup is
// handed to the dispatch loop and this waits for its answer.
func (s *Adapter) NearestAirportLayout(ctx context.Context, lat, lon float64) (*domain.AirportLayout, error) {
	s.mu.RLock()
	queries := s.layoutQueries
	s.mu.RUnlock()
	if queries == nil {
		return nil, domain.ErrNoAirportData
	}
	reply := make(chan layoutResult, 1)
	select {
	case queries <- layoutQuery{lat: lat, lon: lon, reply: reply}:
	case <-ctx.Done():
		return nil, fmt.Errorf("hand lookup to simconnect: %w", ctx.Err())
	}
	select {
	case r := <-reply:
		return r.layout, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for simconnect lookup: %w", ctx.Err())
	}
}

// facilityClient is the part of a SimConnect session an airport lookup
// uses; *sim.SimConnect in the adapter, a fake in tests.
type facilityClient interface {
	AllocDefineID() sim.DWORD
	AddToFacilityDefinition(defineID sim.DWORD, field string) error
	RequestFacilitiesListEX1(listType, requestID sim.DWORD) error
	RequestFacilityData(defineID, requestID sim.DWORD, icao, region string) error
}

// facilityLookup is the airport lookup state of one SimConnect session. It
// is touched only from the dispatch loop's goroutine, so it has no lock.
type facilityLookup struct {
	sc         facilityClient
	runwayDef  sim.DWORD
	parkingDef sim.DWORD
	available  bool
	seq        sim.DWORD

	// The lookup in flight, if any.
	pending  *layoutQuery
	deadline time.Time
	listReq  sim.DWORD
	airports []airportEntry
	pages    map[uint32]bool

	icao        string
	runwayReq   sim.DWORD
	parkingReq  sim.DWORD
	airport     *airportRow
	runways     []runwayRow
	parkings    []parkingRow
	runwaysDone bool
	parkingDone bool
}

// newFacilityLookup registers the two facility definitions. A simulator that
// rejects them (one that predates the Facilities API) leaves the lookup
// unavailable, and every query is answered with ErrNoAirportData.
func newFacilityLookup(sc facilityClient) *facilityLookup {
	f := &facilityLookup{sc: sc}
	f.runwayDef = sc.AllocDefineID()
	f.parkingDef = sc.AllocDefineID()
	for def, fields := range map[sim.DWORD][]string{f.runwayDef: runwayDefinition(), f.parkingDef: parkingDefinition()} {
		for _, field := range fields {
			if err := sc.AddToFacilityDefinition(def, field); err != nil {
				slog.Warn("simulator rejected the airport facility definition; runway and stand will not be reported",
					"error", err)
				return f
			}
		}
	}
	f.available = true
	return f
}

func (f *facilityLookup) nextRequestID() sim.DWORD {
	f.seq++
	return facilityRequestBase + f.seq
}

// start begins a lookup by asking for the airports around the aircraft.
func (f *facilityLookup) start(q layoutQuery) {
	switch {
	case !f.available:
		q.reply <- layoutResult{err: domain.ErrNoAirportData}
		return
	case f.pending != nil:
		q.reply <- layoutResult{err: errFacilityLookupBusy}
		return
	}
	*f = facilityLookup{sc: f.sc, runwayDef: f.runwayDef, parkingDef: f.parkingDef, available: true, seq: f.seq}
	f.pending = &q
	f.deadline = time.Now().Add(facilityLookupTimeout)
	f.listReq = f.nextRequestID()
	f.pages = map[uint32]bool{}
	if err := f.sc.RequestFacilitiesListEX1(sim.FACILITY_LIST_TYPE_AIRPORT, f.listReq); err != nil {
		f.finish(nil, fmt.Errorf("request airport list: %w", err))
	}
}

// expire gives up on a lookup that has run out of time.
func (f *facilityLookup) expire(now time.Time) {
	if f.pending == nil || now.Before(f.deadline) {
		return
	}
	if f.icao == "" {
		// Still waiting for the airport list: no airports around.
		f.finish(nil, domain.ErrNoAirportData)
		return
	}
	f.finish(nil, fmt.Errorf("facility data for %s incomplete after %v", f.icao, facilityLookupTimeout))
}

func (f *facilityLookup) finish(l *domain.AirportLayout, err error) {
	if f.pending != nil {
		f.pending.reply <- layoutResult{layout: l, err: err}
	}
	f.pending = nil
}

// onAirportList collects the pages of the airport list and, once all have
// arrived, asks for the nearest airport's runways and parking.
func (f *facilityLookup) onAirportList(msg []byte) {
	page, err := decodeAirportList(msg)
	if err != nil || f.pending == nil || sim.DWORD(page.RequestID) != f.listReq || f.icao != "" {
		return
	}
	f.airports = append(f.airports, page.Airports...)
	f.pages[page.EntryNumber] = true
	if uint32(len(f.pages)) < page.OutOf {
		return
	}
	nearest, ok := nearestAirport(f.airports, f.pending.lat, f.pending.lon, maxAirportDistanceM)
	if !ok {
		f.finish(nil, domain.ErrNoAirportData)
		return
	}
	f.icao = nearest.ICAO
	f.runwayReq = f.nextRequestID()
	f.parkingReq = f.nextRequestID()
	if err := f.sc.RequestFacilityData(f.runwayDef, f.runwayReq, f.icao, ""); err != nil {
		f.finish(nil, fmt.Errorf("request runways of %s: %w", f.icao, err))
		return
	}
	if err := f.sc.RequestFacilityData(f.parkingDef, f.parkingReq, f.icao, ""); err != nil {
		f.finish(nil, fmt.Errorf("request parking of %s: %w", f.icao, err))
	}
}

// onFacilityData files one row of either reply.
func (f *facilityLookup) onFacilityData(msg []byte) {
	if f.pending == nil || f.icao == "" || len(msg) < recvFacilityDataFixedSize {
		return
	}
	var childSize int
	switch sim.DWORD(u32(msg, 12)) {
	case f.runwayReq:
		childSize = runwayPayloadSize
	case f.parkingReq:
		childSize = parkingPayloadSize
	default:
		return
	}
	row, err := decodeFacilityRow(msg, childSize)
	if err != nil {
		slog.Debug("skipped a facility row", "error", err)
		return
	}
	switch {
	case !row.IsChild:
		ap := readAirportRow(row.Payload)
		f.airport = &ap
	case childSize == runwayPayloadSize:
		f.runways = append(f.runways, readRunwayRow(row.Payload))
	default:
		f.parkings = append(f.parkings, readParkingRow(row.Payload))
	}
}

// onFacilityDataEnd closes one reply and, when both are in, answers.
func (f *facilityLookup) onFacilityDataEnd(requestID sim.DWORD) {
	if f.pending == nil {
		return
	}
	switch requestID {
	case f.runwayReq:
		f.runwaysDone = true
	case f.parkingReq:
		f.parkingDone = true
	default:
		return
	}
	if !f.runwaysDone || !f.parkingDone {
		return
	}
	if f.airport == nil {
		f.finish(nil, fmt.Errorf("facility data for %s had no airport row", f.icao))
		return
	}
	f.finish(buildLayout(f.icao, *f.airport, f.runways, f.parkings), nil)
}

// dispatchFacility routes a facility message to the lookup. It reports
// whether the message was one.
func (f *facilityLookup) dispatchFacility(ppData unsafe.Pointer, recv sim.Recv) bool {
	switch recv.ID {
	case sim.RECV_ID_AIRPORT_LIST:
		f.onAirportList(unsafe.Slice((*byte)(ppData), recv.Size))
	case sim.RECV_ID_FACILITY_DATA:
		f.onFacilityData(unsafe.Slice((*byte)(ppData), recv.Size))
	case sim.RECV_ID_FACILITY_DATA_END:
		f.onFacilityDataEnd((*sim.RecvFacilityDataEnd)(ppData).RequestID)
	default:
		return false
	}
	return true
}
