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

// A revoked or expired token fails the same way on every subsequent poll,
// and nothing else in the adapter would ever tell a caller its token has
// gone bad rather than merely being briefly rejected. OnUnauthorized is the
// one signal that fires so the caller can react instead of retrying forever.
//
// The token matters: it is what makes the 401 mean the session has expired
// rather than that there was never one. See
// TestNoSessionExpiryWhenNoTokenWasSent.
func TestOnUnauthorizedFiresOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthenticated."}`))
	}))
	defer srv.Close()

	a := NewAdapter()
	a.SetBaseURL(srv.URL)
	a.SetToken("a-token-the-server-has-revoked")

	var calls int32
	a.OnUnauthorized(func() { atomic.AddInt32(&calls, 1) })

	_, status, err := a.DoRequest("GET", "/api/v2/acars/messages", nil)
	if err != nil {
		t.Fatalf("DoRequest: %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", status, http.StatusUnauthorized)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("OnUnauthorized callback fired %d times, want 1", got)
	}
}

// A successful reply must never trigger the callback — only an actual 401
// should ever prompt a sign-out.
func TestOnUnauthorizedDoesNotFireOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	a := NewAdapter()
	a.SetBaseURL(srv.URL)

	var calls int32
	a.OnUnauthorized(func() { atomic.AddInt32(&calls, 1) })

	if _, _, err := a.DoRequest("GET", "/api/v2/acars/messages", nil); err != nil {
		t.Fatalf("DoRequest: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("OnUnauthorized callback fired %d times, want 0", got)
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

// unauthorizedServer always rejects, the way the tenant does for a request
// that carries no credentials.
func unauthorizedServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthenticated."}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A pilot reported signing in, being in for about half a second, and landing
// back on the tenant list — having to enter a fresh code afterwards.
//
// The Authorization header is attached only when a token exists, so a call
// made before the token reaches the adapter is rejected exactly like an
// expired one. The session-expired callback signs the pilot out and deletes
// the stored token, so one such request undid a sign-in that had just worked.
func TestNoSessionExpiryWhenNoTokenWasSent(t *testing.T) {
	srv := unauthorizedServer(t)

	a := NewAdapter()
	a.SetBaseURL(srv.URL)

	fired := 0
	a.OnUnauthorized(func() { fired++ })

	if _, status, err := a.DoRequest("GET", "/api/v2/acars/pilot", nil); err != nil {
		t.Fatalf("DoRequest: %v", err)
	} else if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}

	if fired != 0 {
		t.Errorf("signed the pilot out %d time(s) over a request that carried no token; "+
			"a 401 there means sign in, not session expired", fired)
	}
}

// Clearing the token must not make the next call sign the pilot out again.
func TestClearingTheTokenDoesNotCascade(t *testing.T) {
	srv := unauthorizedServer(t)

	a := NewAdapter()
	a.SetBaseURL(srv.URL)
	a.SetToken("dead")

	fired := 0
	a.OnUnauthorized(func() {
		fired++
		a.SetToken("") // what the app does in response
	})

	for i := 0; i < 3; i++ {
		if _, _, err := a.DoRequest("GET", "/api/v2/acars/pilot", nil); err != nil {
			t.Fatalf("DoRequest: %v", err)
		}
	}

	if fired != 1 {
		t.Errorf("callback fired %d times, want 1: once the token is gone the "+
			"remaining calls are unauthenticated, not expired", fired)
	}
}
