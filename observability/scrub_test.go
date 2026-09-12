package observability

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A pilot's bearer token, the device code from the sign-in flow, and the path
// to their Windows profile all pass through this application. None of them help
// debug a crash, and all of them would be a breach to collect.
func TestRedactStringRemovesSecrets(t *testing.T) {
	cases := []struct {
		name string
		in   string
		gone string
	}{
		{
			name: "json web token",
			in:   "auth failed for eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			gone: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		},
		{
			name: "authorization header",
			in:   "Authorization: Bearer sk_live_abcdefghijklmnop",
			gone: "sk_live_abcdefghijklmnop",
		},
		{
			name: "token in a query string",
			in:   "GET https://va.example.com/api/v2/acars/booking?token=abc123def456&page=2",
			gone: "abc123def456",
		},
		{
			name: "long opaque secret",
			in:   "device code AbCdEf0123456789AbCdEf0123456789AbCdEf0123456789 rejected",
			gone: "AbCdEf0123456789AbCdEf0123456789AbCdEf0123456789",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactString(tc.in)
			assert.NotContains(t, got, tc.gone, "the secret survived redaction")
			assert.Contains(t, got, redacted)
		})
	}
}

func TestRedactStringLeavesUsefulTextAlone(t *testing.T) {
	// If redaction eats the diagnosis too, nobody will trust the reports.
	for _, in := range []string{
		"no simulator connected",
		"connect to SimConnect: SimConnect_Open: -1",
		"GET /api/v2/acars/booking returned 503",
		"aircraft Fenix A320 IAE at FL350",
	} {
		assert.Equal(t, in, redactString(in), "redaction should not touch ordinary diagnostics")
	}
}

func TestRedactStringRemovesTheAccountName(t *testing.T) {
	home := t.TempDir() // stands in for the pilot's profile directory
	homePaths = append(homePaths, home)
	t.Cleanup(func() { homePaths = homePaths[:len(homePaths)-1] })

	got := redactString("open " + home + "/airspace-acars/settings.json: permission denied")
	assert.NotContains(t, got, home)
	assert.Contains(t, got, "~/airspace-acars/settings.json")
	assert.Contains(t, got, "permission denied")
}

func TestRedactValueWalksNestedData(t *testing.T) {
	out := redactValue(map[string]any{
		"altitude": 35000,
		"onGround": false,
		"header":   "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl",
		"nested":   map[string]any{"token": "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl"},
	})

	m, ok := out.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 35000, m["altitude"], "numbers are not secrets")
	assert.Equal(t, false, m["onGround"])
	assert.NotContains(t, m["header"], "eyJ")
	assert.NotContains(t, m["nested"].(map[string]any)["token"], "eyJ")
}

func TestRedactValueCleansAnUnknownStructThatCarriesAToken(t *testing.T) {
	type credentials struct{ Token string }
	out := redactValue(credentials{Token: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl"})
	assert.NotContains(t, out.(string), "eyJ", "a struct must not slip through as an opaque value")
}

func TestBeforeSendStripsTheEnvelope(t *testing.T) {
	event := &sentry.Event{
		Message:    "request failed with Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl",
		ServerName: "KEWYN-DESKTOP",
		User:       sentry.User{ID: "42", Email: "pilot@example.com", IPAddress: "203.0.113.7", Name: "A Pilot"},
		Request:    &sentry.Request{URL: "https://va.example.com?token=secret"},
		Exception:  []sentry.Exception{{Value: "token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl rejected"}},
		Breadcrumbs: []*sentry.Breadcrumb{{
			Message: "sent eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl",
			Data:    map[string]any{"url": "https://va.example.com?token=secret"},
		}},
	}

	out := beforeSend(event, nil)
	require.NotNil(t, out)

	assert.NotContains(t, out.Message, "eyJ")
	assert.NotContains(t, out.Exception[0].Value, "eyJ")
	assert.NotContains(t, out.Breadcrumbs[0].Message, "eyJ")
	assert.NotContains(t, out.Breadcrumbs[0].Data["url"], "secret")

	assert.Nil(t, out.Request, "a desktop client has no inbound request to report")
	assert.Equal(t, "desktop", out.ServerName, "the machine name is the pilot's, not ours")
	assert.Equal(t, "42", out.User.ID, "the network identifier is the one thing kept")
	assert.Empty(t, out.User.Email)
	assert.Empty(t, out.User.Name)
	assert.Empty(t, out.User.IPAddress)
}

func TestBeforeSendAttachesTheCounters(t *testing.T) {
	resetCounters()
	Add("position.reports_sent", 41)
	Count("position.reports_failed")
	Gauge("position.outbox_depth", 7)

	out := beforeSend(&sentry.Event{Message: "boom"}, nil)
	require.NotNil(t, out)

	ctx := out.Contexts["counters"]
	require.NotNil(t, ctx, "a report should say what the application had been doing")
	assert.Equal(t, int64(41), ctx["position.reports_sent"])
	assert.Equal(t, int64(1), ctx["position.reports_failed"])
	assert.Equal(t, float64(7), ctx["position.outbox_depth"])
}

func TestCountersAccumulateWhenSentryIsOff(t *testing.T) {
	// Reporting is off in tests, and the counters still have to work: they are
	// read back by beforeSend the moment a DSN is configured.
	resetCounters()
	assert.False(t, Enabled())

	for range 3 {
		Count("sim.reconnect_attempts", "adapter", "SimConnect")
	}
	Add("position.reports_sent", 10)

	assert.Equal(t, map[string]int64{
		"sim.reconnect_attempts": 3,
		"position.reports_sent":  10,
	}, Counters())
}

func TestAddIgnoresNonPositiveAmounts(t *testing.T) {
	resetCounters()
	Add("position.reports_sent", 0)
	Add("position.reports_sent", -5)
	assert.Empty(t, Counters())
}

func TestSpanAndCaptureAreSafeWhenSentryIsOff(t *testing.T) {
	// Every call site runs in a build with no DSN, so none of this may panic
	// or return something a caller has to nil-check.
	ctx, span := Start(t.Context(), "sim.connect", "sim.type", "auto")
	require.NotNil(t, span)
	require.NotNil(t, ctx)
	span.Set("adapter", "SimConnect")
	span.Fail(errors.New("no simulator connected"))
	span.Finish()

	Capture(errors.New("boom"), "where", "test")
	Capture(nil)
	Note("nothing to see")
	SetPilot("https://va.example.com", "42")
	ClearPilot()
}

func TestPairsHandlesAnOddCallSite(t *testing.T) {
	assert.Equal(t, map[string]any{"a": 1, "b": 2}, pairs([]any{"a", 1, "b", 2}))
	assert.Equal(t, map[string]any{"a": 1, "detail": "orphan"}, pairs([]any{"a", 1, "orphan"}),
		"a miscounted call site should lose formatting, not information")
	assert.Nil(t, pairs(nil))
}

func TestBreadcrumbsAreRateLimitedPerCounter(t *testing.T) {
	// The position reporter counts every report it ships. One breadcrumb each
	// would push everything else out of the ring buffer.
	resetCounters()
	for range 500 {
		Count("position.reports_sent")
	}

	countersMu.Lock()
	defer countersMu.Unlock()
	assert.Equal(t, int64(500), counters["position.reports_sent"])
	assert.Equal(t, 1, len(lastCrumb), "only the first occurrence in the window leaves a breadcrumb")
}

func TestRedactedConstantIsObvious(t *testing.T) {
	// Somebody reading a report has to be able to tell redaction from data.
	assert.True(t, strings.HasPrefix(redacted, "["))
}

func resetCounters() {
	countersMu.Lock()
	defer countersMu.Unlock()
	counters = map[string]int64{}
	gauges = map[string]float64{}
	lastCrumb = map[string]time.Time{}
	crumbPending = map[string]int64{}
}

// The header form is the one that bit: a naive pattern redacts the word
// "Bearer" and leaves the credential sitting next to it.
func TestRedactStringHandlesEveryAuthorizationShape(t *testing.T) {
	for _, in := range []string{
		"Authorization: Bearer sk_live_abcdefghijklmnop",
		"authorization=Bearer sk_live_abcdefghijklmnop",
		"Bearer sk_live_abcdefghijklmnop",
		"token: sk_live_abcdefghijklmnop",
		"api_key = sk_live_abcdefghijklmnop",
	} {
		assert.NotContains(t, redactString(in), "sk_live_abcdefghijklmnop", "leaked from %q", in)
	}
}

func TestLogBridgeSkipsWhatWasAlreadyReported(t *testing.T) {
	// Recover sends the panic itself, with a real stack trace, and then logs a
	// line for the file. Reporting that line too would double every panic.
	var record slog.Record
	record = slog.NewRecord(time.Now(), slog.LevelError, "recovered from panic", 0)
	record.Add("panic", "boom", alreadyReported, true)
	assert.True(t, isReported(record))

	plain := slog.NewRecord(time.Now(), slog.LevelError, "reconnect failed", 0)
	plain.Add("error", errors.New("no simulator"))
	assert.False(t, isReported(plain))
}
