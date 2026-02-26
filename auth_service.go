package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/pkg/browser"
)

type AuthService struct {
	mu            sync.RWMutex
	httpClient    *http.Client
	settings      *SettingsService
	tenantBaseURL string
	token         string
}

type Tenant struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	LogoURL *string  `json:"logo_url"`
	Domains []string `json:"domains"`
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

	// Resolve relative logo URLs against the API base URL
	for i, t := range tr.Data {
		if t.LogoURL != nil && len(*t.LogoURL) > 0 && (*t.LogoURL)[0] == '/' {
			full := baseURL + *t.LogoURL
			tr.Data[i].LogoURL = &full
		}
	}

	return tr.Data, nil
}

func (a *AuthService) SelectTenant(domain string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tenantBaseURL = "https://" + domain
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
	a.mu.RLock()
	baseURL := a.tenantBaseURL
	token := a.token
	a.mu.RUnlock()

	if baseURL == "" {
		return nil, 0, fmt.Errorf("no tenant selected")
	}

	var bodyReader io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequest(method, baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}

	return respBody, resp.StatusCode, nil
}
