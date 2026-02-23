package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type AuthService struct {
	mockServerAddr string
}

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error,omitempty"`
}

func (a *AuthService) RequestDeviceCode() (*DeviceCodeResponse, error) {
	resp, err := http.PostForm(
		fmt.Sprintf("http://%s/device/code", a.mockServerAddr),
		url.Values{},
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

func (a *AuthService) PollForToken(deviceCode string) (*TokenResponse, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm(
		fmt.Sprintf("http://%s/device/token", a.mockServerAddr),
		url.Values{"device_code": {deviceCode}},
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
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &tr, nil
}
