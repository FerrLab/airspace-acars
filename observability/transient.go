package observability

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
)

// Transient reports whether err is a network failure rather than a fault in
// the ACARS.
//
// A pilot flies for hours on a home connection, through a CDN that closes
// idle keep-alive connections on its own schedule. Reads reset, DNS blips,
// Wi-Fi drops on the walk to the kitchen. None of that is a defect, none of
// it is actionable, and the ACARS is already built to absorb it: a position
// report that fails goes to the outbox and ships on the next drain.
//
// Reporting these as errors buries the real ones — the same failure mode the
// probe noise caused, where 762 of 767 events were something working as
// designed.
//
// The test is structural rather than a table of error numbers, because the
// numbers do not survive the trip to Windows: syscall.ECONNRESET there is a
// Go-internal value, while a reset socket returns WSAECONNRESET (10054), and
// errors.Is does not bridge the two. Every transport-level failure arrives
// wrapped in a *net.OpError whatever the platform, so that is what is
// matched.
func Transient(err error) bool {
	if err == nil {
		return false
	}

	// http.Client wraps everything it returns; unwrap to the cause.
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		err = urlErr.Err
	}

	// A read, write, or dial that failed at the socket: reset, aborted,
	// refused, unreachable, timed out. The ACARS cannot fix any of them.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	// Name resolution: offline, or a resolver having a bad moment.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// A connection cut mid-response, and our own timeouts.
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	// Anything else still claiming to be a timeout.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return false
}
