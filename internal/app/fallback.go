package app

import "airspace-acars/internal/domain"

// executeCandidates probes candidate paths in order and returns the first
// successful response. It preserves meaningful statuses (accessDenied, rateLimited)
// instead of letting subsequent 404s overwrite them, and halts immediately on
// 401/403 or 429. If one candidate returns a client/server error (400, 422, 5xx),
// testing continues to subsequent candidates.
func executeCandidates(
	candidates []string,
	requestFn func(path string) ([]byte, string, error),
) ([]byte, string, error) {
	var bestBody []byte
	bestStatus := "unavailable"
	var lastErr error

	for _, path := range candidates {
		body, status, err := requestFn(path)
		if err != nil {
			// If this candidate returned a status error (e.g. 400 from v1),
			// record it but allow subsequent candidates (e.g. v2) to be tried.
			lastErr = err
			continue
		}

		if status == "ok" {
			return body, "ok", nil
		}

		if status == "localMode" || status == "noSession" {
			return nil, status, nil
		}

		// 401/403 or 429 is authoritative: stop probing immediately.
		if status == "accessDenied" || status == "rateLimited" {
			return nil, status, nil
		}

		if status != "" {
			bestStatus = status
		}
	}

	if bestStatus == "unavailable" && lastErr != nil {
		return nil, bestStatus, lastErr
	}
	return bestBody, bestStatus, nil
}

// executeRequest performs an authenticated request and maps the HTTP status.
// It returns "ok" for 2xx, "accessDenied" for 401/403, "rateLimited" for 429,
// "unavailable" for 404/405, and returns a StatusError for other non-2xx codes.
func (a *App) executeRequest(path string) ([]byte, string, error) {
	if a.GetSettings().LocalMode {
		return nil, "localMode", nil
	}
	if a.Airspace.Token() == "" || a.Airspace.BaseURL() == "" {
		return nil, "noSession", nil
	}
	body, status, err := a.Airspace.DoRequest("GET", path, nil)
	if err != nil {
		return nil, "", err
	}
	switch status {
	case 200, 201, 202:
		return body, "ok", nil
	case 401, 403:
		return nil, "accessDenied", nil
	case 404, 405:
		return nil, "unavailable", nil
	case 429:
		return nil, "rateLimited", nil
	default:
		return nil, "error", domain.NewStatusError("GET", path, status, body)
	}
}
