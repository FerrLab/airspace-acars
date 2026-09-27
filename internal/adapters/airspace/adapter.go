package airspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	apiKey         string
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

func (a *Adapter) SetAPIKey(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.apiKey = key
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

func (a *Adapter) APIKey() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.apiKey
}

// OnUnauthorized registers a callback fired when the server rejects the
// current token with a 401.
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
	apiKey := a.apiKey
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
		req.Header.Set("Accept", "application/json")

		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if apiKey != "" && strings.HasPrefix(path, "/api/v1/") {
			req.Header.Set("X-API-Key", apiKey)
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		return req, nil
	}

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
	// Authorization header is attached only when a token exists.
	// A private v1 API may require a different credential. Its rejection does
	// not invalidate the pilot's ACARS token (e.g. optional company NOTAMs).
	if resp.StatusCode == http.StatusUnauthorized && token != "" && strings.HasPrefix(path, "/api/v2/acars/") {
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
