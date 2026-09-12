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

// Adapter is the Airspace HTTP API client.
type Adapter struct {
	mu         sync.RWMutex
	httpClient *http.Client
	baseURL    string
	token      string
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

	var bodyReader io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			err = fmt.Errorf("marshal body: %w", err)
			span.Fail(err)
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bodyReader)
	if err != nil {
		err = fmt.Errorf("create request: %w", err)
		span.Fail(err)
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := a.httpClient.Do(req)
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
