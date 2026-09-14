// Package observability wires the application into Sentry.
//
// It replaces an earlier OpenTelemetry setup that wrote traces and metrics to
// JSONL files next to the executable. Those files only helped when a pilot
// thought to send them; Sentry reports a failure the moment it happens. The
// shape of the instrumentation is unchanged — spans around the operations that
// can fail, counters for the events worth knowing about — but the counters are
// now breadcrumbs and a running total attached to each report, because Sentry
// has no home for a free-standing counter.
//
// Everything here is a no-op when no DSN is configured, so a fork or a local
// build reports nothing and the application behaves exactly as before.
package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
)

// DSN is the Sentry project to report to. It is empty in a plain `go build`,
// injected at release time with
//
//	-ldflags "-X airspace-acars/observability.DSN=https://…"
//
// and can be overridden at runtime with AIRSPACE_SENTRY_DSN, which is also how
// a developer points a local build at their own project.
var DSN = ""

// breadcrumbInterval limits how often one counter may leave a breadcrumb. The
// position reporter counts every report it ships, and a breadcrumb each time
// would push everything else out of the ring buffer long before an error had a
// chance to carry it.
const breadcrumbInterval = 30 * time.Second

var (
	enabled bool

	countersMu   sync.Mutex
	counters     = map[string]int64{}
	gauges       = map[string]float64{}
	lastCrumb    = map[string]time.Time{}
	crumbPending = map[string]int64{}
)

// Enabled reports whether events are being sent anywhere.
func Enabled() bool { return enabled }

// Init sets up logging and, when a DSN is configured, Sentry. The returned
// shutdown function flushes anything still queued and must be called on exit.
//
// The log file is kept regardless of Sentry: the Settings window reads it back
// through TailLogs, and it is the only record a pilot can send by hand when
// reporting is switched off.
func Init(serviceName, version string) (shutdown func(context.Context) error, err error) {
	dir, err := exeDir()
	if err != nil {
		return nil, err
	}

	logFile, err := os.OpenFile(filepath.Join(dir, "stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	dsn := strings.TrimSpace(os.Getenv("AIRSPACE_SENTRY_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(DSN)
	}

	if dsn != "" {
		environment := "production"
		if version == "dev" || strings.Contains(version, "beta") {
			environment = "development"
		}

		err = sentry.Init(sentry.ClientOptions{
			Dsn:         dsn,
			Release:     serviceName + "@" + version,
			Environment: environment,

			// A desktop application runs on a pilot's own machine. Nothing
			// that identifies the machine or the person goes out unless the
			// application puts it there deliberately: see SetPilot.
			SendDefaultPII: false,
			ServerName:     "desktop",

			AttachStacktrace: true,
			MaxBreadcrumbs:   100,

			// Errors are the point of this; traces are a sample, because a
			// long-haul flight would otherwise ship a transaction every
			// second of it.
			EnableTracing:    true,
			TracesSampleRate: tracesSampleRate(),

			BeforeSend:       beforeSend,
			BeforeBreadcrumb: beforeBreadcrumb,
		})
		if err != nil {
			logFile.Close()
			return nil, fmt.Errorf("init sentry: %w", err)
		}
		enabled = true

		// The webview reports into the same project; the tag is what tells
		// the two halves apart.
		sentry.ConfigureScope(func(scope *sentry.Scope) {
			scope.SetTag("side", "backend")
		})
	}

	// Errors written to the log are reported as well, so a failure that never
	// reaches a span is still visible.
	slog.SetDefault(slog.New(&sentryHandler{
		next: slog.NewJSONHandler(logFile, nil),
	}))

	if enabled {
		slog.Info("sentry reporting enabled", "release", serviceName+"@"+version)
	}

	return func(ctx context.Context) error {
		if enabled {
			sentry.FlushWithContext(ctx)
		}
		return logFile.Close()
	}, nil
}

// tracesSampleRate reads AIRSPACE_SENTRY_TRACES for a rate between 0 and 1.
// The default is deliberately low: this is a client running on thousands of
// machines, not a server whose quota someone is watching.
func tracesSampleRate() float64 {
	const fallback = 0.05
	raw := strings.TrimSpace(os.Getenv("AIRSPACE_SENTRY_TRACES"))
	if raw == "" {
		return fallback
	}
	var rate float64
	if _, err := fmt.Sscanf(raw, "%f", &rate); err != nil || rate < 0 || rate > 1 {
		return fallback
	}
	return rate
}

// exeDir returns the directory containing the running executable, so the log
// lands next to it however the application was launched.
func exeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// --- Spans ------------------------------------------------------------------

// Span is one timed operation. It is safe to use when Sentry is off, in which
// case every method does nothing.
type Span struct {
	span *sentry.Span
}

// Start begins a span for an operation, carrying optional key/value detail in
// the same shape as slog. A span started on a context that has no span of its
// own becomes a transaction.
func Start(ctx context.Context, op string, kv ...any) (context.Context, *Span) {
	if !enabled {
		return ctx, &Span{}
	}

	var span *sentry.Span
	if sentry.SpanFromContext(ctx) != nil {
		span = sentry.StartSpan(ctx, op)
	} else {
		span = sentry.StartTransaction(ctx, op)
	}

	s := &Span{span: span}
	s.Set(kv...)
	return span.Context(), s
}

// Set attaches detail to the span, in the same key/value shape as Start.
func (s *Span) Set(kv ...any) {
	if s == nil || s.span == nil {
		return
	}
	for k, v := range pairs(kv) {
		s.span.SetData(k, redactValue(v))
	}
}

// Fail marks the operation as failed and reports the error. It replaces the
// RecordError plus SetStatus pair the OpenTelemetry code used to write.
func (s *Span) Fail(err error) {
	if err == nil {
		return
	}
	if s != nil && s.span != nil {
		s.span.Status = sentry.SpanStatusInternalError
		s.span.SetData("error", redactString(err.Error()))
	}
	Capture(err)
}

// Finish closes the span. A span that was never failed is reported as OK.
func (s *Span) Finish() {
	if s == nil || s.span == nil {
		return
	}
	if s.span.Status == sentry.SpanStatusUndefined {
		s.span.Status = sentry.SpanStatusOK
	}
	s.span.Finish()
}

// --- Events -----------------------------------------------------------------

// Capture reports an error, with optional key/value detail attached to it.
func Capture(err error, kv ...any) {
	if err == nil || !enabled {
		return
	}
	detail := pairs(kv)
	if len(detail) == 0 {
		sentry.CaptureException(err)
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetContext("detail", sentry.Context(redactMap(detail)))
		sentry.CaptureException(err)
	})
}

// Note leaves a breadcrumb: no event of its own, but it travels with the next
// error to be reported.
func Note(message string, kv ...any) {
	if !enabled {
		return
	}
	sentry.AddBreadcrumb(&sentry.Breadcrumb{
		Message:   message,
		Level:     sentry.LevelInfo,
		Data:      redactMap(pairs(kv)),
		Timestamp: time.Now(),
	})
}

// Count records that something happened. The running total travels with every
// report as the "counters" context, and the event itself leaves a breadcrumb —
// rate-limited per counter, carrying however many occurrences it stands for.
//
// This is what the OpenTelemetry counters became. Sentry has no place for a
// free-standing counter, but "the position reporter failed 12 times before
// this crash" is exactly the context a report needs.
func Count(name string, kv ...any) { Add(name, 1, kv...) }

// Add records that something happened n times — a batch of position reports
// shipped in one go, say. It is Count with an amount.
func Add(name string, n int64, kv ...any) {
	if n <= 0 {
		return
	}
	countersMu.Lock()
	counters[name] += n
	total := counters[name]
	crumbPending[name] += n
	pending := crumbPending[name]

	due := time.Since(lastCrumb[name]) >= breadcrumbInterval
	if due {
		lastCrumb[name] = time.Now()
		crumbPending[name] = 0
	}
	countersMu.Unlock()

	if !enabled || !due {
		return
	}

	data := redactMap(pairs(kv))
	if data == nil {
		data = map[string]any{}
	}
	data["total"] = total
	if pending > 1 {
		data["occurrences"] = pending
	}

	level := sentry.LevelInfo
	if strings.Contains(name, "fail") || strings.Contains(name, "error") {
		level = sentry.LevelWarning
	}

	sentry.AddBreadcrumb(&sentry.Breadcrumb{
		Category:  "counter",
		Message:   name,
		Level:     level,
		Data:      data,
		Timestamp: time.Now(),
	})
}

// Gauge records the latest value of something that goes up and down — a queue
// depth, a drain duration. It leaves no breadcrumb, because these are sampled
// every tick and would bury everything else; instead the most recent value
// travels with each report, which answers the question a report actually
// raises: how deep was the queue when this broke?
func Gauge(name string, value float64) {
	countersMu.Lock()
	gauges[name] = value
	countersMu.Unlock()
}

// Gauges returns a copy of the latest sampled values.
func Gauges() map[string]float64 {
	countersMu.Lock()
	defer countersMu.Unlock()
	out := make(map[string]float64, len(gauges))
	for k, v := range gauges {
		out[k] = v
	}
	return out
}

// Counters returns a copy of the running totals, for the report context and
// for tests.
func Counters() map[string]int64 {
	countersMu.Lock()
	defer countersMu.Unlock()
	out := make(map[string]int64, len(counters))
	for k, v := range counters {
		out[k] = v
	}
	return out
}

// SetPilot records who is flying, so reports from one pilot can be followed
// across a session. Only the network's own identifier is sent — never a name,
// an email address or the machine's account.
func SetPilot(tenant, pilotID string) {
	if !enabled {
		return
	}
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetUser(sentry.User{ID: pilotID})
		if tenant != "" {
			scope.SetTag("tenant", tenant)
		}
	})
}

// ClearPilot forgets the pilot on sign-out.
func ClearPilot() {
	if !enabled {
		return
	}
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetUser(sentry.User{})
	})
}

// Recover reports a panic and lets the goroutine die quietly rather than
// taking the application with it. Defer it at the top of every goroutine that
// runs for the life of the process.
func Recover() {
	r := recover()
	if r == nil {
		return
	}
	if enabled {
		sentry.CurrentHub().Recover(r)
		sentry.Flush(2 * time.Second)
	}
	// The panic is already on its way to Sentry with a real stack trace; the
	// log line exists for the file, so it is marked not to be reported again.
	slog.Error("recovered from panic", "panic", fmt.Sprint(r), alreadyReported, true)
}

// --- slog bridge ------------------------------------------------------------

// sentryHandler passes every record to the log file and reports the errors.
type sentryHandler struct {
	next  slog.Handler
	attrs []slog.Attr
}

func (h *sentryHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *sentryHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &sentryHandler{next: h.next.WithAttrs(attrs), attrs: append(append([]slog.Attr{}, h.attrs...), attrs...)}
}

func (h *sentryHandler) WithGroup(name string) slog.Handler {
	return &sentryHandler{next: h.next.WithGroup(name), attrs: h.attrs}
}

// alreadyReported marks a log record whose failure has been sent to Sentry by
// some other route, so the log bridge does not report it a second time.
const alreadyReported = "sentry.reported"

func (h *sentryHandler) Handle(ctx context.Context, record slog.Record) error {
	if enabled && record.Level >= slog.LevelError && !isReported(record) {
		h.report(record)
	}
	return h.next.Handle(ctx, record)
}

func isReported(record slog.Record) bool {
	reported := false
	record.Attrs(func(a slog.Attr) bool {
		if a.Key == alreadyReported {
			reported, _ = a.Value.Any().(bool)
			return false
		}
		return true
	})
	return reported
}

// report turns a logged error into a Sentry event. When the record carries an
// error value the event is an exception, which groups by error rather than by
// the message text; otherwise the message itself is the event.
func (h *sentryHandler) report(record slog.Record) {
	extra := map[string]any{}
	for _, a := range h.attrs {
		extra[a.Key] = a.Value.Any()
	}

	var err error
	record.Attrs(func(a slog.Attr) bool {
		if a.Key == alreadyReported {
			return true
		}
		if e, ok := a.Value.Any().(error); ok && err == nil {
			err = e
			return true
		}
		extra[a.Key] = a.Value.Any()
		return true
	})

	sentry.WithScope(func(scope *sentry.Scope) {
		if err != nil {
			// Keep the log message: it says what the application was doing,
			// which the error on its own usually does not.
			extra["log_message"] = record.Message
		}
		scope.SetContext("detail", sentry.Context(redactMap(extra)))
		if err != nil {
			sentry.CaptureException(fmt.Errorf("%s: %w", record.Message, err))
			return
		}
		sentry.CaptureMessage(record.Message)
	})
}

// --- helpers ----------------------------------------------------------------

// pairs turns slog-style alternating keys and values into a map. An odd
// trailing value is kept under a generic key rather than dropped, so a
// mistake at a call site loses formatting but never information.
func pairs(kv []any) map[string]any {
	if len(kv) == 0 {
		return nil
	}
	out := make(map[string]any, (len(kv)+1)/2)
	for i := 0; i < len(kv); i += 2 {
		if i+1 >= len(kv) {
			out["detail"] = kv[i]
			break
		}
		key, ok := kv[i].(string)
		if !ok {
			key = fmt.Sprint(kv[i])
		}
		out[key] = kv[i+1]
	}
	return out
}
