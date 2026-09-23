package app

import (
	"database/sql"
	"encoding/json"
	"sort"
	"sync"
	"testing"
	"time"

	"airspace-acars/internal/domain"
)

// --- stubs -----------------------------------------------------------------

// slowAPI answers every request after a delay, the way a congested link or a
// loaded server does. It records the sample timestamps it was handed.
type slowAPI struct {
	delay time.Duration

	mu         sync.Mutex
	timestamps map[int64]bool
}

func newSlowAPI(delay time.Duration) *slowAPI {
	return &slowAPI{delay: delay, timestamps: map[int64]bool{}}
}

func (s *slowAPI) DoRequest(method, path string, body interface{}) ([]byte, int, error) {
	s.record(body)
	time.Sleep(s.delay)
	return []byte(`{"ok":true}`), 202, nil
}

func (s *slowAPI) record(body interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch v := body.(type) {
	case map[string]interface{}:
		s.note(v)
	case []map[string]interface{}:
		for _, r := range v {
			s.note(r)
		}
	}
}

func (s *slowAPI) note(r map[string]interface{}) {
	if ts, ok := r["timestamp"].(int64); ok {
		s.timestamps[ts] = true
	}
}

// sampled returns the sample timestamps the client handed over.
func (s *slowAPI) sampled() map[int64]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[int64]bool, len(s.timestamps))
	for ts := range s.timestamps {
		out[ts] = true
	}
	return out
}

func (s *slowAPI) SetBaseURL(string)             {}
func (s *slowAPI) SetToken(string)               {}
func (s *slowAPI) BaseURL() string               { return "https://tenant.example" }
func (s *slowAPI) Token() string                 { return "" }
func (s *slowAPI) RawGet(string) ([]byte, error) { return nil, nil }
func (s *slowAPI) OnUnauthorized(func())         {}

// memDB is an in-memory stand-in for the outbox.
type memDB struct {
	mu   sync.Mutex
	next int64
	rows map[int64][]byte
}

func newMemDB() *memDB { return &memDB{rows: map[int64][]byte{}} }

func (d *memDB) EnqueuePosition(bookingID string, payload []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.next++
	d.rows[d.next] = payload
	return nil
}

func (d *memDB) PeekOutboxBatch(bookingID string, limit int) ([]int64, [][]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var ids []int64
	var payloads [][]byte
	for id, raw := range d.rows {
		if len(ids) >= limit {
			break
		}
		ids = append(ids, id)
		payloads = append(payloads, raw)
	}
	return ids, payloads, nil
}

func (d *memDB) DeleteOutboxBatch(ids []int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, id := range ids {
		delete(d.rows, id)
	}
	return nil
}

func (d *memDB) CountOutbox(bookingID string) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.rows), nil
}

// queued returns the sample timestamps sitting in the outbox.
func (d *memDB) queued() map[int64]bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := map[int64]bool{}
	for _, raw := range d.rows {
		var r map[string]interface{}
		if json.Unmarshal(raw, &r) == nil {
			if ts, ok := r["timestamp"].(float64); ok {
				out[int64(ts)] = true
			}
		}
	}
	return out
}

func (d *memDB) SaveFlightData(*domain.FlightData) error { return nil }
func (d *memDB) QueryFlightData() (*sql.Rows, error)     { return nil, nil }
func (d *memDB) PurgeFlightData() error                  { return nil }

// flaringSim reports an aircraft in the flare: below 50 ft, airborne, fast.
type flaringSim struct{}

func (flaringSim) Connect() error          { return nil }
func (flaringSim) Disconnect() error       { return nil }
func (flaringSim) Name() string            { return "Stub" }
func (flaringSim) LastReceived() time.Time { return time.Now() }
func (flaringSim) GetFlightData() (*domain.FlightData, error) {
	fd := &domain.FlightData{}
	fd.Position.AltitudeAGL = 30
	fd.Position.Latitude = 1
	fd.Position.Longitude = 2
	fd.Attitude.GS = 150
	fd.Sensors.OnGround = false
	return fd, nil
}

type nullUI struct{}

func (nullUI) EmitEvent(string, interface{}) {}

// --- the reproduction -------------------------------------------------------

// The reported symptom is a hole: samples stop just below 50 ft and resume
// after touchdown, with ~10 s of the flare missing entirely.
//
// This drives the real loop through a flare while uploads are slow, then looks
// for a hole in the sample timeline. The samples are not lost in transit —
// they are never taken, because the loop that samples at 33 ms is the same one
// that waits on each request, and a ticker drops the ticks it cannot deliver.
func TestFlareIsSampledWithoutHolesWhileUploadsAreSlow(t *testing.T) {
	const (
		uploadDelay = 400 * time.Millisecond
		runFor      = 3 * time.Second

		// A sample every 33 ms. Anything beyond this is a hole, not jitter.
		maxGap = 150 * time.Millisecond
	)

	api := newSlowAPI(uploadDelay)
	db := newMemDB()
	a := &App{Airspace: api, DB: db, UI: nullUI{}}
	a.connector = flaringSim{}
	a.simActive = true
	a.bookingID = "booking-718"
	a.startTime = time.Now()
	a.callsign = "TEST718"

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		a.positionLoop(stop)
		close(done)
	}()

	time.Sleep(runFor)
	close(stop)
	<-done

	// Every sample the client accounted for, delivered or durably queued.
	seen := map[int64]bool{}
	for ts := range api.sampled() {
		seen[ts] = true
	}
	for ts := range db.queued() {
		seen[ts] = true
	}

	stamps := make([]int64, 0, len(seen))
	for ts := range seen {
		stamps = append(stamps, ts)
	}
	sort.Slice(stamps, func(i, j int) bool { return stamps[i] < stamps[j] })

	if len(stamps) < 10 {
		t.Fatalf("only %d samples in total; the loop never reached the flare", len(stamps))
	}

	// High-res starts at the first pair less than 50 ms apart; before that the
	// loop is sampling once a second and wide gaps are correct.
	start := -1
	for i := 1; i < len(stamps); i++ {
		if stamps[i]-stamps[i-1] <= 50 {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("no high-rate samples at all: the flare was never collected")
	}

	worst, worstAt := int64(0), int64(0)
	for i := start; i < len(stamps); i++ {
		if gap := stamps[i] - stamps[i-1]; gap > worst {
			worst, worstAt = gap, stamps[i-1]
		}
	}

	t.Logf("%d samples, %d in the flare, largest gap %d ms", len(stamps), len(stamps)-start+1, worst)
	if worst > maxGap.Milliseconds() {
		t.Errorf("a %d ms hole in the flare after %d (about %d samples never taken); "+
			"sampling must not wait on an upload", worst, worstAt, worst/33)
	}
}
