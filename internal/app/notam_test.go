package app

import (
	"net/url"
	"testing"
)

type notamAPI struct {
	stubAPI
	path string
}

func (*notamAPI) Token() string { return "pilot-token" }
func (s *notamAPI) DoRequest(method, path string, body interface{}) ([]byte, int, error) {
	s.path = path
	return s.stubAPI.DoRequest(method, path, body)
}

func TestNOTAMListPaginationAndQuery(t *testing.T) {
	api := &notamAPI{stubAPI: stubAPI{status: 200, body: []byte(`{"data":[{"id":42,"title":"Operations","content":"Notice","created_at":"2026-09-26"}],"meta":{"current_page":2,"last_page":4}}`)}}
	a := &App{Airspace: api}
	result, err := a.GetNOTAMs(2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || result.CurrentPage != 2 || result.LastPage != 4 || len(result.Data) != 1 || result.Data[0].ID != "42" {
		t.Fatalf("unexpected response: %+v", result)
	}
	u, _ := url.Parse(api.path)
	q := u.Query()
	if u.Path != companyNOTAMPath || q.Get("filter[is_active]") != "1" || q.Get("page") != "2" || q.Get("sort_dir") != "desc" {
		t.Fatalf("wrong request: %s", api.path)
	}
}

func TestNOTAMUnavailableIsNotAnEmptyList(t *testing.T) {
	for status, want := range map[int]string{401: "accessDenied", 403: "accessDenied", 404: "unavailable", 405: "unavailable", 429: "rateLimited"} {
		api := &notamAPI{stubAPI: stubAPI{status: status, body: []byte("not JSON")}}
		a := &App{Airspace: api}
		result, err := a.GetNOTAMs(1)
		if err != nil || result.Status != want {
			t.Fatalf("status %d: %+v, %v", status, result, err)
		}
	}
}

func TestNOTAMDoesNotRequestInLocalModeOrWithoutSession(t *testing.T) {
	api := &notamAPI{}
	a := &App{Airspace: api}
	a.settings.LocalMode = true
	result, err := a.GetNOTAMs(1)
	if err != nil || result.Status != "localMode" || api.calls != 0 {
		t.Fatalf("unexpected local request: %+v %v", result, err)
	}
	noSession := &stubAPI{}
	a = &App{Airspace: noSession}
	result, err = a.GetNOTAMs(1)
	if err != nil || result.Status != "noSession" || noSession.calls != 0 {
		t.Fatalf("unexpected anonymous request: %+v %v", result, err)
	}
}

func TestNOTAMRejectsMalformedSuccess(t *testing.T) {
	for _, body := range []string{`<html>login</html>`, `{}`, `{"data":null}`, `{"data":[{"title":"missing id"}]}`} {
		a := &App{Airspace: &notamAPI{stubAPI: stubAPI{status: 200, body: []byte(body)}}}
		if _, err := a.GetNOTAMs(1); err == nil {
			t.Fatalf("accepted malformed response %s", body)
		}
	}
}

func TestNOTAMDetailAndFlatPagination(t *testing.T) {
	api := &notamAPI{stubAPI: stubAPI{status: 200}}
	a := &App{Airspace: api}
	for _, body := range []string{`{"data":{"id":"7","title":"Notice","body":"Full text"}}`, `{"id":7,"title":"Notice","description":"Full text"}`} {
		api.body = []byte(body)
		detail, err := a.GetNOTAM("7")
		if err != nil || detail.Data == nil || detail.Data.Content != "Full text" {
			t.Fatalf("unexpected detail: %+v %v", detail, err)
		}
	}
	api.body = []byte(`{"data":[],"current_page":1,"last_page":3}`)
	page, err := a.GetNOTAMs(0)
	if err != nil || page.CurrentPage != 1 || page.LastPage != 3 {
		t.Fatalf("unexpected pagination: %+v %v", page, err)
	}
	for _, id := range []string{"", "../pilot", "https://example.com", "%2f"} {
		if _, err := a.GetNOTAM(id); err == nil {
			t.Fatalf("accepted unsafe id %q", id)
		}
	}
}
