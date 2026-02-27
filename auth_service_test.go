package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestAuthService creates an AuthService wired to an httptest.Server.
// The handler receives all requests. Caller must close the returned server.
func newTestAuthService(handler http.HandlerFunc) (*AuthService, *httptest.Server) {
	server := httptest.NewServer(handler)
	settings := &SettingsService{
		settings: Settings{APIBaseURL: server.URL},
	}
	auth := &AuthService{
		httpClient:    server.Client(),
		settings:      settings,
		tenantBaseURL: server.URL,
		token:         "test-token",
	}
	return auth, server
}

func TestDoRequestGET(t *testing.T) {
	auth, server := newTestAuthService(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/api/test", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})
	defer server.Close()

	body, status, err := auth.doRequest("GET", "/api/test", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"ok":true}`, string(body))
}

func TestDoRequestPOST(t *testing.T) {
	auth, server := newTestAuthService(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/submit", r.URL.Path)

		var payload map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "hello", payload["message"])

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":1}`))
	})
	defer server.Close()

	body, status, err := auth.doRequest("POST", "/api/submit", map[string]string{"message": "hello"})
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, status)
	assert.JSONEq(t, `{"id":1}`, string(body))
}

func TestDoRequestNoTenant(t *testing.T) {
	auth := &AuthService{
		httpClient: http.DefaultClient,
		settings:   &SettingsService{},
	}

	_, _, err := auth.doRequest("GET", "/api/test", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tenant selected")
}

func TestDoRequestNoToken(t *testing.T) {
	auth, server := newTestAuthService(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	auth.token = ""

	_, status, err := auth.doRequest("GET", "/api/test", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
}

func TestFetchTenants(t *testing.T) {
	logo := "https://logo.png"
	auth, server := newTestAuthService(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/tenants", r.URL.Path)
		resp := tenantsResponse{
			Data: []Tenant{
				{
					ID:      "1",
					Name:    "Airline Co",
					LogoURL: &logo,
					Domains: []string{"airline.example.com"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	tenants, err := auth.FetchTenants()
	require.NoError(t, err)
	require.Len(t, tenants, 1)
	assert.Equal(t, "Airline Co", tenants[0].Name)
	assert.Equal(t, "1", tenants[0].ID)
	require.NotNil(t, tenants[0].LogoURL)
	assert.Equal(t, "https://logo.png", *tenants[0].LogoURL)
	assert.Equal(t, []string{"airline.example.com"}, tenants[0].Domains)
}

func TestFetchTenantsEmpty(t *testing.T) {
	auth, server := newTestAuthService(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[]}`))
	})
	defer server.Close()

	tenants, err := auth.FetchTenants()
	require.NoError(t, err)
	assert.Empty(t, tenants)
}
