package app

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"airspace-acars/internal/domain"
)

// fixedAPI answers every request the same way and keeps what it was sent.
type fixedAPI struct {
	status int
	err    error

	mu    sync.Mutex
	calls int
	got   [][]map[string]interface{}
}

func (f *fixedAPI) DoRequest(method, path string, body interface{}) ([]byte, int, error) {
	f.mu.Lock()
	f.calls++
	if batch, ok := body.([]map[string]interface{}); ok {
		f.got = append(f.got, batch)
	}
	f.mu.Unlock()

	if f.err != nil {
		return nil, 0, f.err
	}
	// Encode the way the real adapter does, so a value json.Marshal refuses
	// fails here too rather than passing silently.
	if _, err := json.Marshal(body); err != nil {
		return nil, 0, err
	}
	return []byte(`{"ok":true}`), f.status, nil
}

func (f *fixedAPI) attempts() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fixedAPI) SetBaseURL(string)             {}
func (f *fixedAPI) SetToken(string)               {}
func (f *fixedAPI) BaseURL() string               { return "https://tenant.example" }
func (f *fixedAPI) Token() string                 { return "" }
func (f *fixedAPI) RawGet(string) ([]byte, error) { return nil, nil }

func samples(n int) []map[string]interface{} {
	out := make([]map[string]interface{}, n)
	for i := range out {
		out[i] = map[string]interface{}{"timestamp": int64(i), "altitudeAgl": m(float64(i), "ft")}
	}
	return out
}

// quickRetries shortens the backoff so a failure path can be exercised.
func quickRetries(t *testing.T) {
	t.Helper()
	attempts, backoff := uploadAttempts, uploadBackoff
	uploadAttempts, uploadBackoff = 2, time.Millisecond
	t.Cleanup(func() { uploadAttempts, uploadBackoff = attempts, backoff })
}

// --- never drop -------------------------------------------------------------

// A batch that cannot be delivered belongs in the outbox. Dropping it loses
// flight data that nothing will ever ask for again.
func TestFailedUploadGoesToTheOutbox(t *testing.T) {
	quickRetries(t)

	for _, tc := range []struct {
		name string
		api  *fixedAPI
	}{
		{"server error", &fixedAPI{status: 502}},
		{"rejected", &fixedAPI{status: 401}},
		{"network failure", &fixedAPI{err: errors.New("connection reset")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newMemDB()
			a := &App{Airspace: tc.api, DB: db, UI: nullUI{}}
			u := a.startPositionUploader("booking-718")

			u.Submit(samples(40))
			u.Stop()

			if n, _ := db.CountOutbox("booking-718"); n != 40 {
				t.Errorf("outbox holds %d of 40 samples; the rest are gone", n)
			}
			if tc.api.attempts() < 2 {
				t.Errorf("gave up after %d attempt(s) without retrying", tc.api.attempts())
			}
		})
	}
}

func TestDeliveredUploadLeavesNothingQueued(t *testing.T) {
	api := &fixedAPI{status: 202}
	db := newMemDB()
	a := &App{Airspace: api, DB: db, UI: nullUI{}}
	u := a.startPositionUploader("booking-718")

	u.Submit(samples(40))
	u.Stop()

	if n, _ := db.CountOutbox("booking-718"); n != 0 {
		t.Errorf("%d samples queued after a successful upload", n)
	}
}

// A batch larger than the server contract is split, not truncated.
func TestLargeSubmissionIsSplitIntoBatches(t *testing.T) {
	api := &fixedAPI{status: 202}
	db := newMemDB()
	a := &App{Airspace: api, DB: db, UI: nullUI{}}
	u := a.startPositionUploader("booking-718")

	u.Submit(samples(maxBatchSize*2 + 7))
	u.Stop()

	total := 0
	for _, b := range api.got {
		if len(b) > maxBatchSize {
			t.Errorf("a batch of %d exceeds the %d limit", len(b), maxBatchSize)
		}
		total += len(b)
	}
	if total != maxBatchSize*2+7 {
		t.Errorf("%d samples reached the server, want %d", total, maxBatchSize*2+7)
	}
}

// --- NaN --------------------------------------------------------------------

// json.Marshal refuses NaN and ±Inf, and refuses the whole document with it.
// One bad reading from the simulator used to fail a batch of 250 samples — and
// fail again when that batch was written to the outbox, losing all of them.
func TestOneBadReadingDoesNotFailTheBatch(t *testing.T) {
	batch := samples(10)
	batch[4]["attitude"] = map[string]interface{}{
		"vs":     m(math.NaN(), "fpm"),
		"gForce": m(math.Inf(1), "G"),
		"ias":    m(math.Inf(-1), "kts"),
	}

	raw, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("a batch with one unusable reading still fails to encode: %v", err)
	}
	if !strings.Contains(string(raw), `"value":null`) {
		t.Error("the unusable reading should encode as null")
	}

	// The other nine samples must survive intact.
	var back []map[string]interface{}
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("re-reading the batch: %v", err)
	}
	if len(back) != 10 {
		t.Errorf("%d samples survived encoding, want 10", len(back))
	}
}

func TestGoodReadingsAreUnchanged(t *testing.T) {
	raw, err := json.Marshal(m(123.5, "ft"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(raw); got != `{"value":123.5,"unit":"ft"}` {
		t.Errorf("got %s", got)
	}
}

// A NaN must not stop a batch reaching the server, nor stop it reaching the
// outbox when the server is down.
func TestABadReadingStillReachesTheOutboxOnFailure(t *testing.T) {
	quickRetries(t)

	api := &fixedAPI{status: 503}
	db := newMemDB()
	a := &App{Airspace: api, DB: db, UI: nullUI{}}
	u := a.startPositionUploader("booking-718")

	batch := samples(5)
	batch[2]["attitude"] = map[string]interface{}{"vs": m(math.NaN(), "fpm")}

	u.Submit(batch)
	u.Stop()

	if n, _ := db.CountOutbox("booking-718"); n != 5 {
		t.Errorf("outbox holds %d of 5 samples; a NaN should cost one reading, not the batch", n)
	}
}

// --- the upload contract ----------------------------------------------------

// Anything that is not a 2xx is a failure the client must retry. The server
// answers a malformed body with 202 and stores nothing, so a 2xx is the only
// evidence of delivery the client has — and it has to be strict about the rest.
func TestOnlySuccessCountsAsDelivered(t *testing.T) {
	for status, delivered := range map[int]bool{
		200: true, 201: true, 202: true, 204: true,
		301: false, 400: false, 401: false, 404: false,
		500: false, 502: false, 503: false, 504: false,
	} {
		err := domain.NewStatusError("POST", positionPath, status, nil)
		if got := err == nil; got != delivered {
			t.Errorf("status %d treated as delivered=%v, want %v", status, got, delivered)
		}
	}
}
