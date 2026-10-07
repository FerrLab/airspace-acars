//go:build windows

package simconnect

import (
	"errors"
	"testing"
	"time"

	"airspace-acars/internal/domain"
	sim "airspace-acars/internal/simconnect"
)

// fakeFacilities records what a lookup asks the simulator for.
type fakeFacilities struct {
	nextDef   sim.DWORD
	defineErr error
	listReqs  []sim.DWORD
	dataReqs  map[sim.DWORD]sim.DWORD // request ID -> define ID
	dataICAOs []string
}

func (f *fakeFacilities) AllocDefineID() sim.DWORD { f.nextDef++; return f.nextDef }
func (f *fakeFacilities) AddToFacilityDefinition(sim.DWORD, string) error {
	return f.defineErr
}
func (f *fakeFacilities) RequestFacilitiesListEX1(_, requestID sim.DWORD) error {
	f.listReqs = append(f.listReqs, requestID)
	return nil
}
func (f *fakeFacilities) RequestFacilityData(defineID, requestID sim.DWORD, icao, _ string) error {
	if f.dataReqs == nil {
		f.dataReqs = map[sim.DWORD]sim.DWORD{}
	}
	f.dataReqs[requestID] = defineID
	f.dataICAOs = append(f.dataICAOs, icao)
	return nil
}

func (f *fakeFacilities) requestFor(defineID sim.DWORD) uint32 {
	for req, def := range f.dataReqs {
		if def == defineID {
			return uint32(req)
		}
	}
	return 0
}

func ask(f *facilityLookup, lat, lon float64) chan layoutResult {
	reply := make(chan layoutResult, 1)
	f.start(layoutQuery{lat: lat, lon: lon, reply: reply})
	return reply
}

func answered(t *testing.T, reply chan layoutResult) layoutResult {
	t.Helper()
	select {
	case r := <-reply:
		return r
	default:
		t.Fatal("the lookup never answered")
		return layoutResult{}
	}
}

func airportRowMsg(requestID uint32) []byte {
	return facilityDataMsg(true, requestID, 0, (&msgBuilder{}).f64(51.4775).f64(-0.4614).f64(25).b)
}

// A whole lookup: the airport list arrives in two pages, the nearest airport
// is asked for, and its runways and parking come back as one layout.
func TestAnAirportLookupAssemblesTheNearestAirport(t *testing.T) {
	sc := &fakeFacilities{}
	f := newFacilityLookup(sc)
	reply := ask(f, 51.4700, -0.4500)

	list := uint32(sc.listReqs[0])
	f.onAirportList(withRequestID(airportListMsg(9, 0, 2, airportEntry{ICAO: "EGLC", Lat: 51.5053, Lon: 0.0553}), list))
	if len(sc.dataICAOs) != 0 {
		t.Fatal("asked for an airport before the list was complete")
	}
	f.onAirportList(withRequestID(airportListMsg(9, 1, 2, airportEntry{ICAO: "EGLL", Lat: 51.4775, Lon: -0.4614}), list))
	if len(sc.dataICAOs) != 2 || sc.dataICAOs[0] != "EGLL" {
		t.Fatalf("asked for %v, want EGLL's runways and parking", sc.dataICAOs)
	}

	rw, pk := sc.requestFor(f.runwayDef), sc.requestFor(f.parkingDef)
	f.onFacilityData(airportRowMsg(rw))
	f.onFacilityData(facilityDataMsg(true, rw, 42, runwayPayload(runwayRow{
		Lat: 51.4776, Lon: -0.4590, Heading: 90, Length: 3600, Width: 50,
		PrimaryNumber: 9, PrimaryDesignator: 1, SecondaryNumber: 27, SecondaryDesignator: 2})))
	f.onFacilityDataEnd(sim.DWORD(rw))
	select {
	case <-reply:
		t.Fatal("answered before the parking reply was in")
	default:
	}
	f.onFacilityData(airportRowMsg(pk))
	f.onFacilityData(facilityDataMsg(true, pk, 42, parkingPayload(parkingRow{Name: 12, Number: 12, Radius: 30})))
	f.onFacilityDataEnd(sim.DWORD(pk))

	r := answered(t, reply)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.layout.ICAO != "EGLL" || len(r.layout.Runways) != 1 || len(r.layout.Stands) != 1 ||
		r.layout.Runways[0].Ends[0].Designator != "09L" || r.layout.Stands[0].Name != "A12" {
		t.Fatalf("layout = %+v", r.layout)
	}
}

func withRequestID(msg []byte, id uint32) []byte {
	msg[12], msg[13], msg[14], msg[15] = byte(id), byte(id>>8), byte(id>>16), byte(id>>24)
	return msg
}

// Out at sea there is no airport to describe.
func TestNoAirportNearbyIsNoAirportData(t *testing.T) {
	sc := &fakeFacilities{}
	f := newFacilityLookup(sc)
	reply := ask(f, 50.0, -20.0)
	f.onAirportList(withRequestID(airportListMsg(9, 0, 1, airportEntry{ICAO: "EGLL", Lat: 51.4775, Lon: -0.4614}), uint32(sc.listReqs[0])))
	if r := answered(t, reply); !errors.Is(r.err, domain.ErrNoAirportData) {
		t.Fatalf("err = %v, want ErrNoAirportData", r.err)
	}
}

// A list that never comes is an empty facility cache; data that never
// finishes is a failure. Either way the caller gets an answer.
func TestALookupThatIsNeverAnsweredTimesOut(t *testing.T) {
	sc := &fakeFacilities{}
	f := newFacilityLookup(sc)
	reply := ask(f, 51.47, -0.45)
	f.expire(time.Now())
	select {
	case <-reply:
		t.Fatal("gave up before the timeout")
	default:
	}
	f.expire(time.Now().Add(facilityLookupTimeout + time.Second))
	if r := answered(t, reply); !errors.Is(r.err, domain.ErrNoAirportData) {
		t.Fatalf("silent list: err = %v, want ErrNoAirportData", r.err)
	}

	reply = ask(f, 51.47, -0.45)
	f.onAirportList(withRequestID(airportListMsg(9, 0, 1, airportEntry{ICAO: "EGLL", Lat: 51.4775, Lon: -0.4614}), uint32(sc.listReqs[1])))
	f.expire(time.Now().Add(facilityLookupTimeout + time.Second))
	if r := answered(t, reply); r.err == nil || errors.Is(r.err, domain.ErrNoAirportData) {
		t.Fatalf("incomplete data: err = %v, want a failure", r.err)
	}
}

// One lookup at a time; a second is turned away rather than tangled with
// the first.
func TestASecondLookupWaitsItsTurn(t *testing.T) {
	f := newFacilityLookup(&fakeFacilities{})
	ask(f, 51.47, -0.45)
	if r := answered(t, ask(f, 51.47, -0.45)); !errors.Is(r.err, errFacilityLookupBusy) {
		t.Fatalf("err = %v, want errFacilityLookupBusy", r.err)
	}
}

// A simulator without the Facilities API answers every lookup with no data.
func TestASimulatorWithoutFacilitiesHasNoAirportData(t *testing.T) {
	f := newFacilityLookup(&fakeFacilities{defineErr: errors.New("E_FAIL")})
	if r := answered(t, ask(f, 51.47, -0.45)); !errors.Is(r.err, domain.ErrNoAirportData) {
		t.Fatalf("err = %v, want ErrNoAirportData", r.err)
	}
}
