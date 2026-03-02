# Observability Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add full OpenTelemetry observability (traces, metrics, slog bridge) to every service boundary, with stdout exporters swappable to OTLP.

**Architecture:** A thin `observability/` package wraps the OTel SDK (TracerProvider, MeterProvider, slog bridge). Each service gets a tracer and meter from this package. Spans use `context.Background()` — no method signature changes. Existing slog calls auto-inherit trace IDs.

**Tech Stack:** `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/sdk`, stdout exporters, `go.opentelemetry.io/contrib/bridges/otelslog`

---

### Task 1: Add OTel dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Install OTel packages**

Run:
```bash
go get go.opentelemetry.io/otel \
  go.opentelemetry.io/otel/sdk \
  go.opentelemetry.io/otel/sdk/metric \
  go.opentelemetry.io/otel/exporters/stdout/stdouttrace \
  go.opentelemetry.io/otel/exporters/stdout/stdoutmetric \
  go.opentelemetry.io/contrib/bridges/otelslog
```

**Step 2: Verify they're in go.mod**

Run: `grep opentelemetry go.mod`
Expected: 6 direct dependencies listed.

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add OpenTelemetry SDK and exporters"
```

---

### Task 2: Create the `observability` package

**Files:**
- Create: `observability/observability.go`

**Step 1: Write the observability package**

```go
package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Init sets up OpenTelemetry tracing, metrics, and the slog bridge.
// It returns a shutdown function that must be called on app exit (typically via defer).
// To swap to OTLP, replace the stdout exporters in this function.
func Init(serviceName, version string) (shutdown func(context.Context) error, err error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return nil, err
	}

	// --- Tracing ---
	traceFile, err := os.OpenFile("traces.jsonl", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	traceExp, err := stdouttrace.New(stdouttrace.WithWriter(traceFile))
	if err != nil {
		traceFile.Close()
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// --- Metrics ---
	metricFile, err := os.OpenFile("metrics.jsonl", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		traceFile.Close()
		return nil, err
	}
	metricExp, err := stdoutmetric.New(stdoutmetric.WithWriter(metricFile))
	if err != nil {
		traceFile.Close()
		metricFile.Close()
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(30*time.Second))),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	// --- slog bridge ---
	// Replace the default slog handler so that log lines emitted inside an
	// active span automatically include trace_id / span_id fields.
	handler := otelslog.NewHandler(serviceName)
	slog.SetDefault(slog.New(handler))

	shutdown = func(ctx context.Context) error {
		var firstErr error
		if e := tp.Shutdown(ctx); e != nil && firstErr == nil {
			firstErr = e
		}
		if e := mp.Shutdown(ctx); e != nil && firstErr == nil {
			firstErr = e
		}
		closeAll(traceFile, metricFile)
		return firstErr
	}
	return shutdown, nil
}

func closeAll(closers ...io.Closer) {
	for _, c := range closers {
		c.Close()
	}
}

// Tracer returns a named tracer for the given service component.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// Meter returns a named meter for the given service component.
func Meter(name string) metric.Meter {
	return otel.Meter(name)
}
```

**Step 2: Verify it compiles**

Run: `go build ./observability/`
Expected: no errors.

**Step 3: Commit**

```bash
git add observability/
git commit -m "feat: add observability package with OTel tracing, metrics, and slog bridge"
```

---

### Task 3: Wire observability into main.go

**Files:**
- Modify: `main.go:1-15` (imports), `main.go:29-41` (early init)

**Step 1: Add the Init call early in main()**

Add import `"airspace-acars/observability"` and `"context"` to the imports block.

After the single-instance check and before `initDB()`, add:

```go
shutdown, err := observability.Init("airspace-acars", Version)
if err != nil {
    log.Fatal("failed to init observability:", err)
}
defer shutdown(context.Background())
```

**Step 2: Verify the app compiles**

Run: `go build .`
Expected: no errors.

**Step 3: Commit**

```bash
git add main.go
git commit -m "feat: wire observability init into main.go"
```

---

### Task 4: Instrument AuthService — `doRequest` span + metrics

This is the highest-value span: every outgoing API call flows through `doRequest`.

**Files:**
- Modify: `auth_service.go:1-12` (imports), `auth_service.go:166-206` (doRequest)

**Step 1: Add tracer, meter, and metric instruments at package scope**

After the imports in `auth_service.go`, add:

```go
var (
	authTracer          = observability.Tracer("auth")
	authMeter           = observability.Meter("auth")
)
```

These will be used in `doRequest` and also in later tasks for `PollForToken`.

**Step 2: Register metric instruments**

Add a package-level `init()` or declare them alongside the tracer. Because OTel instruments are created lazily, declare them as vars:

```go
var (
	apiRequestsTotal, _    = authMeter.Int64Counter("api.requests_total",
		metric.WithDescription("Total outgoing API requests"))
	apiRequestDuration, _  = authMeter.Float64Histogram("api.request_duration_ms",
		metric.WithDescription("API request duration in milliseconds"))
)
```

Note: import `"airspace-acars/observability"`, `"go.opentelemetry.io/otel/trace"`, `"go.opentelemetry.io/otel/attribute"`, `"go.opentelemetry.io/otel/codes"`, `"go.opentelemetry.io/otel/metric"`, and `"time"` (already imported).

**Step 3: Wrap `doRequest` body with a span**

Replace the `doRequest` method body to add tracing. The new version:

```go
func (a *AuthService) doRequest(method, path string, body interface{}) ([]byte, int, error) {
	ctx, span := authTracer.Start(context.Background(), "auth.do_request",
		trace.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.path", path),
		))
	defer span.End()
	start := time.Now()

	a.mu.RLock()
	baseURL := a.tenantBaseURL
	token := a.token
	a.mu.RUnlock()

	if baseURL == "" {
		err := fmt.Errorf("no tenant selected")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, 0, err
	}

	var bodyReader io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			err = fmt.Errorf("marshal body: %w", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bodyReader)
	if err != nil {
		err = fmt.Errorf("create request: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("do request: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		apiRequestDuration.Record(ctx, float64(time.Since(start).Milliseconds()),
			metric.WithAttributes(attribute.String("http.method", method), attribute.String("http.path", path)))
		apiRequestsTotal.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("http.method", method),
				attribute.String("http.path", path),
				attribute.String("http.status", "error")))
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("read response: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, resp.StatusCode, err
	}

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	elapsed := float64(time.Since(start).Milliseconds())
	statusStr := fmt.Sprintf("%d", resp.StatusCode)
	apiRequestDuration.Record(ctx, elapsed,
		metric.WithAttributes(attribute.String("http.method", method), attribute.String("http.path", path)))
	apiRequestsTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.path", path),
			attribute.String("http.status", statusStr)))

	return respBody, resp.StatusCode, nil
}
```

**Step 4: Verify it compiles and tests pass**

Run: `go build . && go test -run TestDoRequest -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add auth_service.go
git commit -m "feat: instrument AuthService.doRequest with OTel span and metrics"
```

---

### Task 5: Instrument AuthService — `PollForToken` span

**Files:**
- Modify: `auth_service.go:110-150` (PollForToken)

**Step 1: Add span to PollForToken**

At the start of `PollForToken`, add:

```go
_, span := authTracer.Start(context.Background(), "auth.poll_for_token")
defer span.End()
```

Before each error return, add:
```go
span.RecordError(err)
span.SetStatus(codes.Error, err.Error())
```

On success, add:
```go
span.SetAttributes(attribute.String("oauth.status", "success"))
```

**Step 2: Verify**

Run: `go build .`
Expected: no errors.

**Step 3: Commit**

```bash
git add auth_service.go
git commit -m "feat: instrument AuthService.PollForToken with OTel span"
```

---

### Task 6: Instrument FlightService — lifecycle spans

**Files:**
- Modify: `flight_service.go:1-11` (imports), `flight_service.go:58-108` (StartFlight), `flight_service.go:110-131` (StopFlight), `flight_service.go:133-164` (FinishFlight)

**Step 1: Add tracer and meter at package scope**

```go
var (
	flightTracer = observability.Tracer("flight")
	flightMeter  = observability.Meter("flight")
)
```

Add imports: `"airspace-acars/observability"`, `"context"`, `"go.opentelemetry.io/otel/trace"`, `"go.opentelemetry.io/otel/attribute"`, `"go.opentelemetry.io/otel/codes"`, `"go.opentelemetry.io/otel/metric"`.

**Step 2: Instrument StartFlight**

At the very top of `StartFlight`, before the sim validation:

```go
_, span := flightTracer.Start(context.Background(), "flight.start",
    trace.WithAttributes(
        attribute.String("flight.callsign", callsign),
        attribute.String("flight.departure", departure),
        attribute.String("flight.arrival", arrival),
    ))
defer span.End()
```

Before each error return, add:
```go
span.RecordError(err)
span.SetStatus(codes.Error, err.Error())
```

(For the inline errors like `fmt.Errorf("simulator not connected")`, assign to a variable first.)

**Step 3: Instrument StopFlight**

Same pattern — create span at top:

```go
_, span := flightTracer.Start(context.Background(), "flight.stop")
defer span.End()
```

After `f.mu.Lock()` and before the guard, set attributes:
```go
span.SetAttributes(attribute.String("flight.callsign", f.callsign))
```

Record errors if request fails.

**Step 4: Instrument FinishFlight**

Same pattern:

```go
_, span := flightTracer.Start(context.Background(), "flight.finish")
defer span.End()
```

After locking, set attributes:
```go
span.SetAttributes(
    attribute.String("flight.callsign", f.callsign),
    attribute.Float64("flight.duration_sec", time.Since(f.startTime).Seconds()),
)
```

**Step 5: Verify**

Run: `go build . && go test -run TestStartFlight -v`
Expected: PASS.

**Step 6: Commit**

```bash
git add flight_service.go
git commit -m "feat: instrument FlightService lifecycle (start/stop/finish) with OTel spans"
```

---

### Task 7: Instrument FlightService — `doRequestWithRetry` span

**Files:**
- Modify: `flight_service.go:194-210` (doRequestWithRetry)

**Step 1: Add span**

```go
func (f *FlightService) doRequestWithRetry(method, path string, body interface{}) ([]byte, int, error) {
	_, span := flightTracer.Start(context.Background(), "flight.request_with_retry",
		trace.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.path", path),
		))
	defer span.End()

	var lastErr error
	backoff := 2 * time.Second
	for attempt := range retryAttempts {
		respBody, status, err := f.auth.doRequest(method, path, body)
		if err == nil {
			span.SetAttributes(
				attribute.Int("retry.attempt", attempt+1),
				attribute.String("retry.final_status", "success"),
			)
			return respBody, status, nil
		}
		lastErr = err
		if attempt < retryAttempts-1 {
			slog.Warn("request failed, retrying", "method", method, "path", path, "attempt", attempt+1, "error", err, "backoff", backoff)
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	span.SetAttributes(
		attribute.Int("retry.attempt", retryAttempts),
		attribute.String("retry.final_status", "failed"),
	)
	span.RecordError(lastErr)
	span.SetStatus(codes.Error, lastErr.Error())
	return nil, 0, fmt.Errorf("all %d attempts failed: %w", retryAttempts, lastErr)
}
```

**Step 2: Verify**

Run: `go build . && go test -run TestDoRequestWithRetry -v 2>&1 | head -20`
Expected: compiles. (Test may or may not exist yet.)

**Step 3: Commit**

```bash
git add flight_service.go
git commit -m "feat: instrument FlightService.doRequestWithRetry with OTel span"
```

---

### Task 8: Instrument FlightService — `positionLoop` metrics and span

**Files:**
- Modify: `flight_service.go:212-308` (positionLoop + flushPendingReports)

**Step 1: Declare metric instruments at package scope**

```go
var (
	posReportsSent, _    = flightMeter.Int64Counter("position.reports_sent",
		metric.WithDescription("Successfully sent position reports"))
	posReportsQueued, _  = flightMeter.Int64Counter("position.reports_queued",
		metric.WithDescription("Position reports queued due to failure"))
	posReportsFailed, _  = flightMeter.Int64Counter("position.reports_failed",
		metric.WithDescription("Position reports that failed to send"))
	posQueueDepth, _     = flightMeter.Int64Histogram("position.queue_depth",
		metric.WithDescription("Position report queue depth at each tick"))
	posFlushTotal, _     = flightMeter.Int64Counter("position.flush_total",
		metric.WithDescription("Flush attempts on flight end"))
)
```

**Step 2: Add metrics to positionLoop**

Inside the `case <-ticker.C:` block, after deciding to queue vs send:

After `pendingReports = pendingReports[sent:]` (line ~269):
```go
posReportsSent.Add(context.Background(), int64(sent))
```

After `pendingReports = append(pendingReports, report)` (line ~280):
```go
posReportsQueued.Add(context.Background(), 1)
```

At the top of the tick case (after getting `fd`), record queue depth:
```go
posQueueDepth.Record(context.Background(), int64(len(pendingReports)))
```

On successful send of current report (the `} else {` at line ~287):
```go
posReportsSent.Add(context.Background(), 1)
```

**Step 3: Add metrics to flushPendingReports**

On successful flush:
```go
posFlushTotal.Add(context.Background(), 1, metric.WithAttributes(attribute.Bool("success", true)))
posReportsSent.Add(context.Background(), int64(len(pending)))
```

On failed flush:
```go
posFlushTotal.Add(context.Background(), 1, metric.WithAttributes(attribute.Bool("success", false)))
```

**Step 4: Verify**

Run: `go build . && go test -run TestPositionLoop -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add flight_service.go
git commit -m "feat: add position report metrics (sent, queued, failed, queue_depth, flush)"
```

---

### Task 9: Instrument FlightDataService — sim connection spans + metrics

**Files:**
- Modify: `flight_data_service.go:1-15` (imports), `flight_data_service.go:49-121` (ConnectSim), `flight_data_service.go:123-141` (DisconnectSim), `flight_data_service.go:146-180` (reconnectSim), `flight_data_service.go:358-474` (dataStreamLoop staleness)

**Step 1: Add tracer, meter, metric instruments**

```go
var (
	simTracer                = observability.Tracer("sim")
	simMeter                 = observability.Meter("sim")
	simReconnectAttempts, _  = simMeter.Int64Counter("sim.reconnect_attempts",
		metric.WithDescription("Simulator reconnection attempts"))
	simStalenessDetected, _  = simMeter.Int64Counter("sim.staleness_detected",
		metric.WithDescription("Stale simulator connection events"))
)
```

Add imports: `"airspace-acars/observability"`, `"context"`, `"go.opentelemetry.io/otel/trace"`, `"go.opentelemetry.io/otel/attribute"`, `"go.opentelemetry.io/otel/codes"`, `"go.opentelemetry.io/otel/metric"`.

**Step 2: Instrument ConnectSim**

At the top of `ConnectSim`:

```go
_, span := simTracer.Start(context.Background(), "sim.connect",
    trace.WithAttributes(attribute.String("sim.type", simType)))
defer span.End()
```

Set `span.SetAttributes(attribute.String("sim.adapter", connector.Name()))` after the connector is created.

On error returns: `span.RecordError(err); span.SetStatus(codes.Error, err.Error())`

**Step 3: Instrument DisconnectSim**

```go
_, span := simTracer.Start(context.Background(), "sim.disconnect")
defer span.End()
```

After lock, set `span.SetAttributes(attribute.String("sim.adapter", f.adapterName))`.

**Step 4: Instrument reconnectSim**

```go
_, span := simTracer.Start(context.Background(), "sim.reconnect",
    trace.WithAttributes(attribute.String("sim.adapter", name)))
defer span.End()
```

On error: `span.RecordError(err); span.SetStatus(codes.Error, err.Error())`

**Step 5: Add metrics to dataStreamLoop staleness block**

When staleness is detected (line ~452), add:
```go
simStalenessDetected.Add(context.Background(), 1,
    metric.WithAttributes(attribute.String("adapter", adapterName)))
```

When reconnectSim is called (success or fail), add:
```go
simReconnectAttempts.Add(context.Background(), 1,
    metric.WithAttributes(
        attribute.String("adapter", adapterName),
        attribute.Bool("success", err == nil)))
```

**Step 6: Verify**

Run: `go build . && go test -run TestFlightData -v`
Expected: PASS.

**Step 7: Commit**

```bash
git add flight_data_service.go
git commit -m "feat: instrument FlightDataService (connect/disconnect/reconnect) with OTel spans and metrics"
```

---

### Task 10: Instrument ChatService

**Files:**
- Modify: `chat_service.go:1-6` (imports), `chat_service.go:33-45` (GetMessages), `chat_service.go:47-68` (SendMessage)

**Step 1: Add tracer**

```go
var chatTracer = observability.Tracer("chat")
```

Add imports: `"airspace-acars/observability"`, `"context"`, `"go.opentelemetry.io/otel/trace"`, `"go.opentelemetry.io/otel/attribute"`, `"go.opentelemetry.io/otel/codes"`.

**Step 2: Instrument GetMessages**

```go
func (c *ChatService) GetMessages(page int) (*MessagesResponse, error) {
	_, span := chatTracer.Start(context.Background(), "chat.get_messages",
		trace.WithAttributes(attribute.Int("chat.page", page)))
	defer span.End()

	// ... existing code ...
	// on error: span.RecordError(err); span.SetStatus(codes.Error, err.Error())
}
```

**Step 3: Instrument SendMessage**

```go
func (c *ChatService) SendMessage(message string) (*ChatMessage, error) {
	_, span := chatTracer.Start(context.Background(), "chat.send_message")
	defer span.End()

	// ... existing code ...
	// on error: span.RecordError(err); span.SetStatus(codes.Error, err.Error())
}
```

**Step 4: Verify**

Run: `go build .`
Expected: no errors.

**Step 5: Commit**

```bash
git add chat_service.go
git commit -m "feat: instrument ChatService with OTel spans"
```

---

### Task 11: Instrument AudioService

**Files:**
- Modify: `audio_service.go:1-15` (imports), `audio_service.go:51-75` (FetchSoundInstructions)

**Step 1: Add tracer**

```go
var audioTracer = observability.Tracer("audio")
```

Add imports: `"airspace-acars/observability"`, `"context"`, `"go.opentelemetry.io/otel/codes"`.

**Step 2: Instrument FetchSoundInstructions**

```go
func (a *AudioService) FetchSoundInstructions() ([]SoundInstruction, error) {
	_, span := audioTracer.Start(context.Background(), "audio.fetch_instructions")
	defer span.End()

	// ... existing code ...
	// on error: span.RecordError(err); span.SetStatus(codes.Error, err.Error())
}
```

**Step 3: Verify**

Run: `go build .`
Expected: no errors.

**Step 4: Commit**

```bash
git add audio_service.go
git commit -m "feat: instrument AudioService with OTel span"
```

---

### Task 12: Instrument UpdateService

**Files:**
- Modify: `update_service.go:1-14` (imports), `update_service.go:88-143` (CheckForUpdate)

**Step 1: Add tracer**

```go
var updateTracer = observability.Tracer("update")
```

Add imports: `"airspace-acars/observability"`, `"go.opentelemetry.io/otel/trace"`, `"go.opentelemetry.io/otel/attribute"`, `"go.opentelemetry.io/otel/codes"`.

**Step 2: Instrument CheckForUpdate**

```go
func (s *UpdateService) CheckForUpdate() (*UpdateInfo, error) {
	_, span := updateTracer.Start(context.Background(), "update.check",
		trace.WithAttributes(attribute.String("update.current_version", Version)))
	defer span.End()

	// ... existing code ...
	// on error: span.RecordError(err); span.SetStatus(codes.Error, err.Error())
	// on success: span.SetAttributes(attribute.Bool("update.available", info.UpdateAvailable))
}
```

**Step 3: Verify**

Run: `go build .`
Expected: no errors.

**Step 4: Commit**

```bash
git add update_service.go
git commit -m "feat: instrument UpdateService with OTel span"
```

---

### Task 13: Instrument DiscordService

**Files:**
- Modify: `discord_service.go:1-13` (imports), `discord_service.go:191-220` (connect), `discord_service.go:254-268` (setActivity)

**Step 1: Add tracer**

```go
var discordTracer = observability.Tracer("discord")
```

Add imports: `"airspace-acars/observability"`, `"context"`, `"go.opentelemetry.io/otel/codes"`.

**Step 2: Instrument connect**

```go
func (d *DiscordService) connect() error {
	_, span := discordTracer.Start(context.Background(), "discord.connect")
	defer span.End()

	// ... existing code ...
	// on error (return fmt.Errorf("no discord pipe found")):
	//   span.RecordError(err); span.SetStatus(codes.Error, err.Error())
}
```

**Step 3: Instrument setActivity**

```go
func (d *DiscordService) setActivity(activity map[string]interface{}) error {
	_, span := discordTracer.Start(context.Background(), "discord.set_activity")
	defer span.End()

	// ... existing code ...
	// on error: span.RecordError(err); span.SetStatus(codes.Error, err.Error())
}
```

**Step 4: Verify**

Run: `go build .`
Expected: no errors.

**Step 5: Commit**

```bash
git add discord_service.go
git commit -m "feat: instrument DiscordService with OTel spans"
```

---

### Task 14: Run full test suite and final verification

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: all PASS.

**Step 2: Run the app briefly to verify traces/metrics files are created**

Run: `go build . && ls -la traces.jsonl metrics.jsonl 2>/dev/null`
Expected: files exist after app startup (may need a quick run).

**Step 3: Final commit if any fixups needed**

```bash
git add -A
git commit -m "chore: fixups from full test suite run"
```
