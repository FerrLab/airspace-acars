package observability

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"

	"airspace-acars/internal/domain"
)

// wsaeconnreset is what a reset socket returns on Windows. It is spelled as a
// number because syscall.WSAECONNRESET only exists on Windows, and the point
// of the test is to run this case on CI, which does not.
const wsaeconnreset = syscall.Errno(10054)

// windowsReset rebuilds the error chain from the live reports: an http.Client
// error wrapping a socket read that the far end closed.
func windowsReset(method, path string) error {
	return fmt.Errorf("do request: %w", &url.Error{
		Op:  method,
		URL: "https://roraima.airspace.ferrlab.com" + path,
		Err: &net.OpError{
			Op:  "read",
			Net: "tcp",
			Err: os.NewSyscallError("wsarecv", wsaeconnreset),
		},
	})
}

func TestTransientMatchesReportedFailures(t *testing.T) {
	// Both of these were reported as errors worth a pilot's attention.
	// Neither is: the ACARS queues the report and ships it on the next drain.
	tests := []struct {
		name string
		err  error
	}{
		{"booking fetch reset", windowsReset("Get", "/api/v2/acars/booking")},
		{"position report reset", windowsReset("Post", "/api/v2/acars/position")},
		{"connection refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}},
		{"dns failure", &net.DNSError{Err: "no such host", Name: "roraima.airspace.ferrlab.com"}},
		{"truncated response", fmt.Errorf("read response: %w", io.ErrUnexpectedEOF)},
		{"client timeout", fmt.Errorf("do request: %w", context.DeadlineExceeded)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !Transient(tt.err) {
				t.Errorf("Transient(%v) = false, want true: this is the network, not a defect", tt.err)
			}
		})
	}
}

// The point of classifying is to keep real faults visible. Silencing the whole
// error path would be worse than the noise it removes.
func TestTransientLeavesRealFaultsReportable(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"no tenant selected", errors.New("no tenant selected")},
		{"malformed response", fmt.Errorf("parse booking: %w", errors.New("invalid character 'x'"))},
		{"untrusted certificate", fmt.Errorf("do request: %w", &url.Error{
			Op:  "Get",
			URL: "https://roraima.airspace.ferrlab.com/api/v2/acars/booking",
			Err: x509.UnknownAuthorityError{},
		})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if Transient(tt.err) {
				t.Errorf("Transient(%v) = true, want false: this one needs reporting", tt.err)
			}
		})
	}
}

// --- Reporting policy -------------------------------------------------------

// captureTransport records what would have been sent to Sentry.
type captureTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (c *captureTransport) SendEvent(e *sentry.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *captureTransport) Flush(time.Duration) bool              { return true }
func (c *captureTransport) FlushWithContext(context.Context) bool { return true }
func (c *captureTransport) Configure(sentry.ClientOptions)        {}
func (c *captureTransport) Close()                                {}

func (c *captureTransport) captured() []*sentry.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*sentry.Event(nil), c.events...)
}

// withSentry points the package at a transport the test can inspect.
func withSentry(t *testing.T) *captureTransport {
	t.Helper()

	transport := &captureTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       "https://key@example.invalid/1",
		Transport: transport,
	})
	if err != nil {
		t.Fatalf("sentry.NewClient: %v", err)
	}

	hub := sentry.NewHub(client, sentry.NewScope())
	previousHub, previousEnabled := sentry.CurrentHub(), enabled
	sentry.SetHubOnContext(context.Background(), hub)
	sentry.CurrentHub().BindClient(client)
	enabled = true

	t.Cleanup(func() {
		enabled = previousEnabled
		sentry.CurrentHub().BindClient(previousHub.Client())
	})
	return transport
}

// The whole point of the change: a dropped connection must not become an
// event, while a genuine fault still does.
func TestFailDoesNotReportNetworkFailures(t *testing.T) {
	transport := withSentry(t)

	span := &Span{}
	span.Fail(windowsReset("Get", "/api/v2/acars/booking"))
	span.Fail(windowsReset("Post", "/api/v2/acars/position"))

	if got := transport.captured(); len(got) != 0 {
		t.Fatalf("captured %d events, want 0: %v", len(got), got[0].Message)
	}
}

func TestFailStillReportsRealFaults(t *testing.T) {
	transport := withSentry(t)

	span := &Span{}
	span.Fail(errors.New("parse booking: invalid character 'x'"))

	if got := transport.captured(); len(got) != 1 {
		t.Fatalf("captured %d events, want 1 — a real fault must still be reported", len(got))
	}
}

// A 5xx says of itself that it will pass. Transient has to honour that, or a
// CDN outage fills the issue feed with events nobody can act on.
func TestTransientHonoursServerFailures(t *testing.T) {
	temporary := domain.NewStatusError("GET", "/api/v2/acars/messages?page=1", 502, []byte("error code: 502"))
	if !Transient(fmt.Errorf("fetch messages: %w", temporary)) {
		t.Error("a 502 should not be reported as a fault")
	}

	// A rejected token is about this request and must stay visible.
	permanent := domain.NewStatusError("GET", "/api/v2/acars/pilot", 401, nil)
	if Transient(permanent) {
		t.Error("a 401 should still be reported")
	}
}
