package airspace

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"airspace-acars/observability"
)

// resetOnceServer answers the first request by killing the connection the way
// a CDN does when it drops a keep-alive — an RST, which surfaces to the client
// as the "connection was forcibly closed" error the live reports carry — then
// serves normally.
func resetOnceServer(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()

	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			// Linger 0 turns Close into an RST rather than a FIN.
			if tcp, ok := conn.(*net.TCPConn); ok {
				_ = tcp.SetLinger(0)
			}
			conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	return srv, &requests
}

func TestGetRetriesADroppedConnection(t *testing.T) {
	srv, requests := resetOnceServer(t)

	a := NewAdapter()
	a.SetBaseURL(srv.URL)

	body, status, err := a.DoRequest("GET", "/api/v2/acars/booking", nil)
	if err != nil {
		t.Fatalf("DoRequest: %v — a dropped connection should have been retried", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q, want the response from the retry", body)
	}
	if got := atomic.LoadInt32(requests); got != 2 {
		t.Errorf("server saw %d requests, want 2 (the reset one and the retry)", got)
	}
}

// A POST may have been processed before the connection died, so retrying it
// risks filing something twice. The caller's outbox is what replays these.
func TestPostIsNotRetried(t *testing.T) {
	srv, requests := resetOnceServer(t)

	a := NewAdapter()
	a.SetBaseURL(srv.URL)

	_, _, err := a.DoRequest("POST", "/api/v2/acars/position", map[string]any{"lat": 1.0})
	if err == nil {
		t.Fatal("DoRequest succeeded; a POST must not be retried after a reset")
	}
	if !observability.Transient(err) {
		t.Errorf("error %v is not classified transient, so it would be reported to Sentry", err)
	}
	if got := atomic.LoadInt32(requests); got != 1 {
		t.Errorf("server saw %d requests, want 1 (no retry)", got)
	}
}

func TestPostBodyStillReachesTheServer(t *testing.T) {
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		got = buf
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := NewAdapter()
	a.SetBaseURL(srv.URL)

	if _, _, err := a.DoRequest("POST", "/api/v2/acars/position", map[string]any{"lat": 1.5}); err != nil {
		t.Fatalf("DoRequest: %v", err)
	}
	if string(got) != `{"lat":1.5}` {
		t.Errorf("server received %q, want the marshalled body", got)
	}
}
