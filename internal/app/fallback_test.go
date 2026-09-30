package app

import (
	"errors"
	"testing"
)

func TestExecuteCandidates(t *testing.T) {
	t.Run("first 200 stops candidates and returns ok", func(t *testing.T) {
		calls := 0
		candidates := []string{"/api/v1/a", "/api/v2/b", "/api/v2/c"}
		body, status, err := executeCandidates(candidates, func(path string) ([]byte, string, error) {
			calls++
			if path == "/api/v1/a" {
				return []byte("data"), "ok", nil
			}
			return nil, "unavailable", nil
		})
		if err != nil || status != "ok" || string(body) != "data" {
			t.Fatalf("expected ok with data, got status=%s err=%v", status, err)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("401 stops immediately and preserves accessDenied over subsequent 404", func(t *testing.T) {
		calls := 0
		candidates := []string{"/api/v1/a", "/api/v2/b", "/api/v2/c"}
		_, status, err := executeCandidates(candidates, func(path string) ([]byte, string, error) {
			calls++
			if path == "/api/v1/a" {
				return nil, "accessDenied", nil
			}
			return nil, "unavailable", nil
		})
		if err != nil || status != "accessDenied" {
			t.Fatalf("expected accessDenied, got status=%s err=%v", status, err)
		}
		if calls != 1 {
			t.Errorf("expected candidate probing to halt on 401, made %d calls", calls)
		}
	})

	t.Run("429 stops immediately and preserves rateLimited", func(t *testing.T) {
		calls := 0
		candidates := []string{"/api/v1/a", "/api/v2/b"}
		_, status, err := executeCandidates(candidates, func(path string) ([]byte, string, error) {
			calls++
			if path == "/api/v1/a" {
				return nil, "rateLimited", nil
			}
			return nil, "unavailable", nil
		})
		if err != nil || status != "rateLimited" {
			t.Fatalf("expected rateLimited, got status=%s err=%v", status, err)
		}
		if calls != 1 {
			t.Errorf("expected candidate probing to halt on 429, made %d calls", calls)
		}
	})

	t.Run("400 on first candidate does not abort second candidate", func(t *testing.T) {
		candidates := []string{"/api/v1/legacy", "/api/v2/recommended"}
		body, status, err := executeCandidates(candidates, func(path string) ([]byte, string, error) {
			if path == "/api/v1/legacy" {
				return nil, "error", errors.New("bad request 400")
			}
			return []byte("v2-data"), "ok", nil
		})
		if err != nil || status != "ok" || string(body) != "v2-data" {
			t.Fatalf("expected ok from v2 candidate, got status=%s err=%v", status, err)
		}
	})

	t.Run("all 404s report unavailable", func(t *testing.T) {
		candidates := []string{"/api/v1/a", "/api/v2/b"}
		_, status, err := executeCandidates(candidates, func(path string) ([]byte, string, error) {
			return nil, "unavailable", nil
		})
		if err != nil || status != "unavailable" {
			t.Fatalf("expected unavailable, got status=%s err=%v", status, err)
		}
	})
}

func TestParseRawID(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "string id", raw: `"doc_42"`, want: "doc_42"},
		{name: "numeric id", raw: `101`, want: "101"},
		{name: "large numeric id", raw: `9223372036854775807`, want: "9223372036854775807"},
		{name: "null id", raw: `null`, want: ""},
		{name: "empty id", raw: ``, want: ""},
		{name: "whitespace padded string", raw: `"  abc  "`, want: "abc"},
		{name: "invalid object id", raw: `{"id":1}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRawID([]byte(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseRawID(%s) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseRawID(%s) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

