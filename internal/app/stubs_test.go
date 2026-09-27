package app

// stubAPI answers every request with one canned status. It exists to carry the
// AirspaceAPI boilerplate for tests that care about one method and not the
// rest; embed it and override what matters.
type stubAPI struct {
	status int
	body   []byte
	calls  int
	apiKey string
}

func (s *stubAPI) DoRequest(method, path string, body interface{}) ([]byte, int, error) {
	s.calls++
	return s.body, s.status, nil
}

func (s *stubAPI) SetBaseURL(string)             {}
func (s *stubAPI) SetToken(string)               {}
func (s *stubAPI) SetAPIKey(k string)            { s.apiKey = k }
func (s *stubAPI) BaseURL() string               { return "https://tenant.example" }
func (s *stubAPI) Token() string                 { return "" }
func (s *stubAPI) APIKey() string                { return s.apiKey }
func (s *stubAPI) RawGet(string) ([]byte, error) { return nil, nil }
func (s *stubAPI) OnUnauthorized(func())         {}
