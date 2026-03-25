package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"

	"github.com/pkg/browser"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var authTracer = observability.Tracer("auth")

// SetToken stores the bearer token for API requests.
func (a *App) SetToken(token string) {
	a.Airspace.SetToken(token)
}

// FetchTenants lists available tenants from the API base URL.
func (a *App) FetchTenants() ([]domain.Tenant, error) {
	a.settingsMu.RLock()
	baseURL := a.settings.APIBaseURL
	a.settingsMu.RUnlock()

	body, err := a.Airspace.RawGet(baseURL + "/api/tenants")
	if err != nil {
		return nil, fmt.Errorf("fetch tenants: %w", err)
	}

	var tr struct {
		Data []domain.Tenant `json:"data"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return tr.Data, nil
}

// SelectTenant sets the tenant base URL for API requests.
func (a *App) SelectTenant(d string) {
	a.Airspace.SetBaseURL(fmt.Sprintf("https://%s", d))
}

// RequestDeviceCode initiates the device-code OAuth flow.
func (a *App) RequestDeviceCode() (*domain.DeviceCodeResponse, error) {
	baseURL := a.Airspace.BaseURL()
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

	var dcr domain.DeviceCodeResponse
	if err := json.Unmarshal(body, &dcr); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &dcr, nil
}

// PollForToken polls the token endpoint during device-code auth flow.
func (a *App) PollForToken(authorizationToken string) (*domain.TokenResponse, error) {
	_, span := authTracer.Start(context.Background(), "auth.poll_for_token")
	defer span.End()

	baseURL := a.Airspace.BaseURL()
	if baseURL == "" {
		err := fmt.Errorf("no tenant selected")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
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
		err = fmt.Errorf("poll token: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("read response: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	var tr domain.TokenResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(body, &tr); err != nil {
			err = fmt.Errorf("parse response: %w", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
	}
	tr.Status = resp.StatusCode

	span.SetAttributes(attribute.String("oauth.status", fmt.Sprintf("%d", resp.StatusCode)))
	return &tr, nil
}

// OpenAuthorizationURL opens the browser to the authorization page.
func (a *App) OpenAuthorizationURL(userCode string) error {
	baseURL := a.Airspace.BaseURL()
	if baseURL == "" {
		return fmt.Errorf("no tenant selected")
	}
	url := fmt.Sprintf("%s/acars/authorize?code=%s", baseURL, userCode)
	return browser.OpenURL(url)
}
