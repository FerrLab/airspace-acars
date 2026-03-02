package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/pkg/browser"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"airspace-acars/observability"
)

var (
	authTracer            = observability.Tracer("auth")
	authMeter             = observability.Meter("auth")
	apiRequestsTotal, _   = authMeter.Int64Counter("api.requests_total",
		metric.WithDescription("Total outgoing API requests"))
	apiRequestDuration, _ = authMeter.Float64Histogram("api.request_duration_ms",
		metric.WithDescription("API request duration in milliseconds"))
)

type AuthService struct {
	mu            sync.RWMutex
	httpClient    *http.Client
	settings      *SettingsService
	tenantBaseURL string
	token         string
}

type Tenant struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	LogoURL   *string  `json:"logo_url"`
	BannerURL *string  `json:"banner_url"`
	Domains   []string `json:"domains"`
}

type tenantsResponse struct {
	Data []Tenant `json:"data"`
}

type DeviceCodeResponse struct {
	UserCode           string `json:"user_code"`
	AuthorizationToken string `json:"authorization_token"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token,omitempty"`
	Status      int    `json:"status"`
	Error       string `json:"error,omitempty"`
}

func (a *AuthService) SetToken(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.token = token
}

func (a *AuthService) FetchTenants() ([]Tenant, error) {
	baseURL := a.settings.GetSettings().APIBaseURL
	resp, err := a.httpClient.Get(baseURL + "/api/tenants")
	if err != nil {
		return nil, fmt.Errorf("fetch tenants: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var tr tenantsResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return tr.Data, nil
}

func (a *AuthService) SelectTenant(domain string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tenantBaseURL = fmt.Sprintf("https://%s", domain)
}

func (a *AuthService) RequestDeviceCode() (*DeviceCodeResponse, error) {
	a.mu.RLock()
	baseURL := a.tenantBaseURL
	a.mu.RUnlock()

	if baseURL == "" {
		return nil, fmt.Errorf("no tenant selected")
	}

	resp, err := a.httpClient.Post(
		baseURL+"/api/v2/acars/auth/request",
		"application/json",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var dcr DeviceCodeResponse
	if err := json.Unmarshal(body, &dcr); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &dcr, nil
}

func (a *AuthService) PollForToken(authorizationToken string) (*TokenResponse, error) {
	a.mu.RLock()
	baseURL := a.tenantBaseURL
	a.mu.RUnlock()

	if baseURL == "" {
		return nil, fmt.Errorf("no tenant selected")
	}

	payload, err := json.Marshal(map[string]string{
		"authorization_token": authorizationToken,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	resp, err := a.httpClient.Post(
		baseURL+"/api/v2/acars/auth/token",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("poll token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var tr TokenResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(body, &tr); err != nil {
			return nil, fmt.Errorf("parse response: %w", err)
		}
	}
	tr.Status = resp.StatusCode

	return &tr, nil
}

func (a *AuthService) OpenAuthorizationURL(userCode string) error {
	a.mu.RLock()
	baseURL := a.tenantBaseURL
	a.mu.RUnlock()

	if baseURL == "" {
		return fmt.Errorf("no tenant selected")
	}
	url := fmt.Sprintf("%s/acars/authorize?code=%s", baseURL, userCode)
	return browser.OpenURL(url)
}

// doRequest makes an authenticated HTTP request to the tenant API.
// Used internally by other services in the same package.
func (a *AuthService) doRequest(method, path string, body interface{}) ([]byte, int, error) {
	ctx, span := authTracer.Start(context.Background(), "auth.do_request",
		trace.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.path", path),
		))
	defer span.End()

	start := time.Now()

	a.mu.RLock()
	baseURL := a.tenantBaseURL
	token := a.token
	a.mu.RUnlock()

	if baseURL == "" {
		err := fmt.Errorf("no tenant selected")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, 0, err
	}

	var bodyReader io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			err = fmt.Errorf("marshal body: %w", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bodyReader)
	if err != nil {
		err = fmt.Errorf("create request: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("do request: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		durationMs := float64(time.Since(start).Milliseconds())
		apiRequestDuration.Record(ctx, durationMs,
			metric.WithAttributes(
				attribute.String("http.method", method),
				attribute.String("http.path", path),
				attribute.String("status", "error"),
			))
		apiRequestsTotal.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("http.method", method),
				attribute.String("http.path", path),
				attribute.String("status", "error"),
			))
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("read response: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		durationMs := float64(time.Since(start).Milliseconds())
		apiRequestDuration.Record(ctx, durationMs,
			metric.WithAttributes(
				attribute.String("http.method", method),
				attribute.String("http.path", path),
				attribute.String("status", "error"),
			))
		apiRequestsTotal.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("http.method", method),
				attribute.String("http.path", path),
				attribute.String("status", "error"),
			))
		return nil, resp.StatusCode, err
	}

	statusStr := fmt.Sprintf("%d", resp.StatusCode)
	span.SetAttributes(attribute.String("http.status_code", statusStr))
	durationMs := float64(time.Since(start).Milliseconds())
	apiRequestDuration.Record(ctx, durationMs,
		metric.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.path", path),
			attribute.String("status", statusStr),
		))
	apiRequestsTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.path", path),
			attribute.String("status", statusStr),
		))

	return respBody, resp.StatusCode, nil
}
