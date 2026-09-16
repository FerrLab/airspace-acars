package app

import (
	"errors"
	"testing"

	"airspace-acars/internal/domain"
)

// stubAPI answers every request with one canned status.
type stubAPI struct {
	status int
	body   []byte
	calls  int
}

func (s *stubAPI) DoRequest(method, path string, body interface{}) ([]byte, int, error) {
	s.calls++
	return s.body, s.status, nil
}

func (s *stubAPI) SetBaseURL(string)             {}
func (s *stubAPI) SetToken(string)               {}
func (s *stubAPI) BaseURL() string               { return "https://tenant.example" }
func (s *stubAPI) Token() string                 { return "" }
func (s *stubAPI) RawGet(string) ([]byte, error) { return nil, nil }

func reports(n int) []map[string]interface{} {
	out := make([]map[string]interface{}, n)
	for i := range out {
		out[i] = map[string]interface{}{"lat": float64(i)}
	}
	return out
}

// A non-2xx reply carries no transport error, so DoRequest returns err == nil.
// sendPositionBatches used to read that as success and advance its cursor,
// discarding the high-res queue the caller would otherwise have persisted.
func TestPositionBatchesAreNotSentOnAServerError(t *testing.T) {
	for _, status := range []int{500, 502, 503, 400, 401} {
		api := &stubAPI{status: status, body: []byte("error code: 502")}
		a := &App{Airspace: api}

		sent, err := a.sendPositionBatches(reports(5))
		if err == nil {
			t.Errorf("status %d: sendPositionBatches reported success", status)
		}
		if sent != 0 {
			t.Errorf("status %d: reported %d sent; nothing was accepted", status, sent)
		}

		var se *domain.StatusError
		if !errors.As(err, &se) || se.Status != status {
			t.Errorf("status %d: error does not name the status: %v", status, err)
		}
	}
}

func TestPositionBatchesSucceedOnAccepted(t *testing.T) {
	// The tenant answers position reports with 202.
	api := &stubAPI{status: 202}
	a := &App{Airspace: api}

	sent, err := a.sendPositionBatches(reports(3))
	if err != nil {
		t.Fatalf("sendPositionBatches: %v", err)
	}
	if sent != 3 {
		t.Errorf("sent = %d, want 3", sent)
	}
}

// The reports that fail are the ones worth keeping, and a 5xx is exactly when
// the outbox has to hold them rather than report them as a fault.
func TestServerErrorsOnPositionsAreNotRaisedAsFaults(t *testing.T) {
	api := &stubAPI{status: 502}
	a := &App{Airspace: api}

	_, err := a.sendPositionBatches(reports(1))
	var se *domain.StatusError
	if !errors.As(err, &se) {
		t.Fatalf("expected a StatusError, got %v", err)
	}
	if !se.Temporary() {
		t.Error("a 502 on position reports should be temporary, so the outbox holds them quietly")
	}
}
