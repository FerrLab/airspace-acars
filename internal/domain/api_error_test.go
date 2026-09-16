package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestStatusErrorIgnoresSuccess(t *testing.T) {
	for _, status := range []int{200, 201, 202, 204, 299} {
		if err := NewStatusError("POST", "/api/v2/acars/position", status, nil); err != nil {
			t.Errorf("status %d produced %v, want nil", status, err)
		}
	}
}

// The reported crash: Cloudflare answers a 502 with the plain text
// "error code: 502", which is not JSON and does not start with '<', so the
// old HTML guard missed it and json.Unmarshal reported the letter 'e'.
func TestStatusErrorNamesTheStatusNotTheBody(t *testing.T) {
	err := NewStatusError("GET", "/api/v2/acars/messages?page=1", 502, []byte("error code: 502"))
	if err == nil {
		t.Fatal("502 produced no error")
	}
	msg := err.Error()
	for _, want := range []string{"502", "/api/v2/acars/messages"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not mention %q", msg, want)
		}
	}
}

func TestStatusErrorTruncatesAnErrorPage(t *testing.T) {
	page := "<html>" + strings.Repeat("x", 5000) + "</html>"
	err := NewStatusError("GET", "/api/v2/acars/booking", 503, []byte(page))
	if n := len(err.Error()); n > 400 {
		t.Errorf("message is %d bytes; an error page should be truncated", n)
	}
	if strings.Contains(err.Error(), "\n") {
		t.Error("message contains a newline")
	}
}

// A 5xx is the server or its CDN having trouble; the pilot cannot act and the
// outbox absorbs it. A 4xx is about this request and stays reportable.
func TestOnlyServerFailuresAreTemporary(t *testing.T) {
	cases := map[int]bool{
		400: false, 401: false, 403: false, 404: false, 422: false,
		500: true, 502: true, 503: true, 504: true,
	}
	for status, want := range cases {
		err := NewStatusError("GET", "/x", status, nil)
		var se *StatusError
		if !errors.As(err, &se) {
			t.Fatalf("status %d did not produce a *StatusError", status)
		}
		if got := se.Temporary(); got != want {
			t.Errorf("status %d Temporary() = %v, want %v", status, got, want)
		}
	}
}
