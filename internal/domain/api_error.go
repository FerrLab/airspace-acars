package domain

import (
	"fmt"
	"strings"
)

// StatusError is a reply the server sent that was not a success.
//
// It exists because a non-2xx reply is not a transport error: the request
// reached the server and came back, so the HTTP client reports no error and a
// caller that only checks err treats the reply as good. When the body is then
// unmarshalled, a CDN's plain-text "error code: 502" surfaces as a JSON syntax
// error, which says nothing about what actually happened; and when the body is
// ignored, a position report the server never accepted is counted as sent.
type StatusError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("%s %s: server returned %d", e.Method, e.Path, e.Status)
	}
	return fmt.Sprintf("%s %s: server returned %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// Temporary reports whether the server is expected to recover on its own.
//
// A 5xx is the server or the CDN in front of it having trouble, which the
// pilot cannot act on and the outbox already absorbs. A 4xx is about this
// request — an expired token, a rejected payload — and stays reportable.
//
// observability.Transient looks for this method, so a 5xx is recorded and
// left as a breadcrumb rather than raised as a fault.
func (e *StatusError) Temporary() bool { return e.Status >= 500 }

// NewStatusError returns an error for a non-2xx reply, or nil for a success.
// The body is included for context and truncated, since an error page can be a
// whole document.
func NewStatusError(method, path string, status int, body []byte) error {
	if status >= 200 && status < 300 {
		return nil
	}
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 200 {
		snippet = snippet[:200] + "…"
	}
	// A newline in the middle of an error page makes the message unreadable.
	snippet = strings.Join(strings.Fields(snippet), " ")
	return &StatusError{Method: method, Path: path, Status: status, Body: snippet}
}
