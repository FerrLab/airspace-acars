package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"

	"github.com/pkg/browser"
)

// SetToken stores the bearer token for API requests.
//
// The token itself is never logged, here or anywhere: only whether there is
// one. That is the part that matters when a pilot reports being signed out,
// and it is the whole of what the log needs to show.
func (a *App) SetToken(token string) {
	a.Airspace.SetToken(token)
	if token == "" {
		observability.ClearPilot()
		slog.Info("auth: token cleared", "tenant", a.Airspace.BaseURL())
		return
	}
	slog.Info("auth: token set", "tenant", a.Airspace.BaseURL())
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
	previous := a.Airspace.BaseURL()
	a.Airspace.SetBaseURL(fmt.Sprintf("https://%s", d))
	if previous != "" && previous != fmt.Sprintf("https://%s", d) {
		// Worth its own line: a token belongs to one tenant, so a switch is
		// where a request can end up at the wrong one.
		slog.Info("auth: tenant switched", "from", previous, "to", d)
		return
	}
	slog.Info("auth: tenant selected", "tenant", d)
}

// RequestDeviceCode initiates the device-code OAuth flow.
func (a *App) RequestDeviceCode() (*domain.DeviceCodeResponse, error) {
	baseURL := a.Airspace.BaseURL()
	if baseURL == "" {
		return nil, fmt.Errorf("no tenant selected")
	}

	slog.Info("auth: requesting a device code", "tenant", baseURL)

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
	_, span := observability.Start(context.Background(), "auth.poll_for_token")
	defer span.Finish()

	baseURL := a.Airspace.BaseURL()
	if baseURL == "" {
		err := fmt.Errorf("no tenant selected")
		span.Fail(err)
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
		span.Fail(err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("read response: %w", err)
		span.Fail(err)
		return nil, err
	}

	var tr domain.TokenResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(body, &tr); err != nil {
			err = fmt.Errorf("parse response: %w", err)
			span.Fail(err)
			return nil, err
		}
	}
	tr.Status = resp.StatusCode

	// The poll runs on a timer while the pilot is in the browser, so only the
	// outcomes are logged: pending is the expected answer nearly every time.
	switch {
	case resp.StatusCode == http.StatusOK:
		slog.Info("auth: device code accepted, signing in", "tenant", baseURL)
	case resp.StatusCode >= 400 && resp.StatusCode != http.StatusAccepted:
		slog.Warn("auth: device code rejected", "tenant", baseURL, "status", resp.StatusCode)
	}

	span.Set("oauth.status", fmt.Sprintf("%d", resp.StatusCode))
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
