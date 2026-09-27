package app

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"airspace-acars/internal/domain"
)

const companyNOTAMPath = "/api/v1/company-notams"

// This is the documented company API. A tenant may reject the pilot token;
// that means this feature is unavailable, not that the ACARS session expired.
func (a *App) notamRequest(path string) ([]byte, string, error) {
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
	slog.Info("notamRequest", "path", path, "status", status)
	switch status {
	case 401, 403:
		return nil, "accessDenied", nil
	case 404, 405:
		return nil, "unavailable", nil
	case 429:
		return nil, "rateLimited", nil
	}
	if err := domain.NewStatusError("GET", path, status, body); err != nil {
		return nil, "", err
	}
	return body, "ok", nil
}

// The manual documents pagination but not the complete notice schema. Accept
// the common text fields without exposing arbitrary server HTML to the webview.
type notamWire struct {
	ID          json.RawMessage `json:"id"`
	Title       string          `json:"title"`
	Content     string          `json:"content"`
	Body        string          `json:"body"`
	Description string          `json:"description"`
	CreatedAt   string          `json:"created_at"`
}

func (n notamWire) notice() (domain.CompanyNOTAM, error) {
	var id string
	if json.Unmarshal(n.ID, &id) != nil {
		var number json.Number
		if err := json.Unmarshal(n.ID, &number); err != nil {
			return domain.CompanyNOTAM{}, fmt.Errorf("invalid NOTAM id")
		}
		id = number.String()
	}
	if id == "" {
		return domain.CompanyNOTAM{}, fmt.Errorf("missing NOTAM id")
	}
	content := n.Content
	if content == "" {
		content = n.Body
	}
	if content == "" {
		content = n.Description
	}
	return domain.CompanyNOTAM{ID: id, Title: n.Title, Content: content, CreatedAt: n.CreatedAt}, nil
}

func (a *App) GetNOTAMs(page int) (*domain.NOTAMPage, error) {
	if page < 1 {
		page = 1
	}
	query := url.Values{
		"page": {strconv.Itoa(page)}, "per_page": {"25"},
		"filter[is_active]": {"1"}, "sort": {"created_at"}, "sort_dir": {"desc"},
	}

	candidates := []string{
		companyNOTAMPath + "?" + query.Encode(),
		"/api/v2/acars/notams?" + query.Encode(),
		"/api/v2/acars/company-notams?" + query.Encode(),
	}

	body, status, err := executeCandidates(candidates, a.notamRequest)
	if err != nil {
		return nil, err
	}

	result := &domain.NOTAMPage{Status: status, Data: []domain.CompanyNOTAM{}, CurrentPage: page, LastPage: page}
	if status != "ok" {
		return result, nil
	}
	var response struct {
		Data        *[]notamWire `json:"data"`
		CurrentPage int          `json:"current_page"`
		LastPage    int          `json:"last_page"`
		Meta        struct {
			CurrentPage int `json:"current_page"`
			LastPage    int `json:"last_page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Data == nil {
		return nil, fmt.Errorf("invalid NOTAM list response")
	}
	for _, item := range *response.Data {
		notice, err := item.notice()
		if err != nil {
			return nil, err
		}
		result.Data = append(result.Data, notice)
	}
	if response.Meta.CurrentPage > 0 {
		result.CurrentPage = response.Meta.CurrentPage
	} else if response.CurrentPage > 0 {
		result.CurrentPage = response.CurrentPage
	}
	if response.Meta.LastPage > 0 {
		result.LastPage = response.Meta.LastPage
	} else if response.LastPage > 0 {
		result.LastPage = response.LastPage
	}
	return result, nil
}

func (a *App) GetNOTAM(id string) (*domain.NOTAMDetail, error) {
	// IDs are a single path segment, never a URL supplied by the server/UI.
	if id == "" || strings.ContainsAny(id, "/\\?#%") || id == "." || id == ".." {
		return nil, fmt.Errorf("invalid NOTAM id")
	}

	candidates := []string{
		companyNOTAMPath + "/" + url.PathEscape(id),
		"/api/v2/acars/notams/" + url.PathEscape(id),
		"/api/v2/acars/company-notams/" + url.PathEscape(id),
	}

	body, status, err := executeCandidates(candidates, a.notamRequest)
	if err != nil {
		return nil, err
	}

	result := &domain.NOTAMDetail{Status: status}
	if status != "ok" {
		return result, nil
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("invalid NOTAM detail response")
	}
	if len(envelope.Data) > 0 {
		body = envelope.Data
	}
	var wire notamWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("invalid NOTAM detail response")
	}
	notice, err := wire.notice()
	if err != nil {
		return nil, err
	}
	result.Data = &notice
	return result, nil
}
