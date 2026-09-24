package airspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"airspace-acars/observability"
)

// retryDelay is the pause before a retried request. Long enough for a stale
// pooled connection to be discarded, short enough that nobody notices.
const retryDelay = 250 * time.Millisecond

// Adapter is the Airspace HTTP API client.
type Adapter struct {
	mu             sync.RWMutex
	httpClient     *http.Client
	baseURL        string
	token          string
	onUnauthorized func()
}

// NewAdapter creates a new Airspace API adapter.
func NewAdapter() *Adapter {
	return &Adapter{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Adapter) SetBaseURL(url string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.baseURL = url
}

func (a *Adapter) SetToken(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.token = token
}

func (a *Adapter) BaseURL() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.baseURL
}

func (a *Adapter) Token() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.token
}

// OnUnauthorized registers a callback fired when the server rejects the
// current token with a 401.
//
// A revoked or expired token otherwise fails silently on every poll, forever:
// the frontend has no way to learn the token has gone bad, so it keeps
// believing it is signed in and keeps retrying with the same dead token every
// few seconds — each attempt its own reportable error. The callback exists so
// the caller can sign the pilot out and prompt a fresh login the moment this
// is first detected, rather than only ever finding out from the error stream.
func (a *Adapter) OnUnauthorized(fn func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onUnauthorized = fn
}

// DoRequest makes an authenticated HTTP request to baseURL + path.
func (a *Adapter) DoRequest(method, path string, body interface{}) ([]byte, int, error) {
	ctx, span := observability.Start(context.Background(), "auth.do_request",
		"http.method", method,
		"http.path", path)
	defer span.Finish()

	a.mu.RLock()
	baseURL := a.baseURL
	token := a.token
	a.mu.RUnlock()

	if baseURL == "" {
		err := fmt.Errorf("no tenant selected")
		span.Fail(err)
		return nil, 0, err
	}

	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			err = fmt.Errorf("marshal body: %w", err)
			span.Fail(err)
			return nil, 0, err
		}
	}

	// Built per attempt: a request body is read once, so a retry needs its
	// own reader over the same bytes.
	newRequest := func() (*http.Request, error) {
		var bodyReader io.Reader
		if payload != nil {
			bodyReader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bodyReader)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return req, nil
	}

	// The tenant sits behind a CDN that closes idle keep-alive connections on
	// its own schedule, so the first request after a quiet spell can go out
	// on a socket that is already gone. One retry on a fresh connection turns
	// that into a hiccup instead of an error the pilot sees.
	//
	// Only idempotent methods are retried. A POST that failed while reading
	// the response may well have been processed, and the ACARS must not risk
	// filing anything twice — position reports have an outbox that replays
	// them properly.
	attempts := 1
	if method == http.MethodGet || method == http.MethodHead {
		attempts = 2
	}

	var resp *http.Response
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		var req *http.Request
		if req, err = newRequest(); err != nil {
			err = fmt.Errorf("create request: %w", err)
			span.Fail(err)
			return nil, 0, err
		}

		if resp, err = a.httpClient.Do(req); err == nil {
			break
		}
		if attempt == attempts || !observability.Transient(err) {
			break
		}
		observability.Note("retrying after a dropped connection",
			"http.method", method, "http.path", path)
		time.Sleep(retryDelay)
	}

	if err != nil {
		err = fmt.Errorf("do request: %w", err)
		span.Fail(err)
		observability.Count("api.requests_total", "http.method", method, "http.path", path, "status", "error")
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("read response: %w", err)
		span.Fail(err)
		observability.Count("api.requests_total", "http.method", method, "http.path", path, "status", "error")
		return nil, resp.StatusCode, err
	}

	statusStr := fmt.Sprintf("%d", resp.StatusCode)
	span.Set("http.status_code", statusStr)
	observability.Count("api.requests_total", "http.method", method, "http.path", path, "status", statusStr)

	// A 401 only means the session has expired if there was a session: the
	// Authorization header is attached only when a token exists, so a request
	// made before one is set — or after one was just cleared — is rejected the
	// same way. Treating that as an expired session signed the pilot out of a
	// tenant they had just signed in to, and deleted the stored token on the
	// way out, so the next attempt needed a fresh code as well.
	if resp.StatusCode == http.StatusUnauthorized && token != "" {
		a.mu.RLock()
		cb := a.onUnauthorized
		a.mu.RUnlock()
		if cb != nil {
			cb()
		}
	}

	return respBody, resp.StatusCode, nil
}

// RawGet performs an unauthenticated GET to an absolute URL.
func (a *Adapter) RawGet(url string) ([]byte, error) {
	resp, err := a.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return body, nil
}
