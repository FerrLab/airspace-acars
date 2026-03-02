# Observability Design: Full OpenTelemetry Integration

**Date:** 2026-03-01
**Status:** Approved

## Goal

Add comprehensive observability (logs, traces, metrics) to every service boundary using OpenTelemetry, with stdout/file exporters now and a one-line swap path to OTLP/Sentry later.

## Architecture

### `observability/observability.go`

A thin wrapper around the OTel SDK that centralizes initialization.

**Exports:**
- `Init(serviceName, version string) (shutdown func(context.Context), err error)` — sets up TracerProvider (stdout), MeterProvider (stdout, 30s periodic reader), and slog-OTel bridge
- `Shutdown(ctx context.Context)` — flushes and closes all providers
- `Tracer(name string) trace.Tracer` — returns a named tracer
- `Meter(name string) metric.Meter` — returns a named meter

**Swapping exporters:** Change `Init()` internals to use `otlptracehttp`/`otlpmetrichttp`. No other files change.

### Integration Pattern

- Each service declares `var tracer = observability.Tracer("service_name")` and `var meter = observability.Meter("service_name")` at package level
- Spans use `context.Background()` (no signature changes to existing methods)
- Existing `slog.Info/Warn/Error` calls remain unchanged; the slog bridge auto-attaches trace/span IDs

## Span Points (17 total)

### AuthService
| Span | Attributes |
|------|-----------|
| `auth.do_request` | `http.method`, `http.path`, `http.status_code` |
| `auth.poll_for_token` | `oauth.status` |

### FlightService
| Span | Attributes |
|------|-----------|
| `flight.start` | `flight.callsign`, `flight.departure`, `flight.arrival` |
| `flight.stop` | `flight.callsign` |
| `flight.finish` | `flight.callsign`, `flight.duration_sec` |
| `flight.request_with_retry` | `http.method`, `http.path`, `retry.attempt`, `retry.final_status` |
| `flight.position_send` | `position.queue_depth`, `position.interval_ms` |
| `flight.flush_pending` | `flush.count`, `flush.success` |

### FlightDataService
| Span | Attributes |
|------|-----------|
| `sim.connect` | `sim.adapter`, `sim.type` |
| `sim.reconnect` | `sim.adapter`, `sim.attempt` |
| `sim.disconnect` | `sim.adapter` |

### ChatService
| Span | Attributes |
|------|-----------|
| `chat.get_messages` | `chat.page` |
| `chat.send_message` | — |

### AudioService
| Span | Attributes |
|------|-----------|
| `audio.fetch_instructions` | — |

### UpdateService
| Span | Attributes |
|------|-----------|
| `update.check` | `update.current_version` |

### DiscordService
| Span | Attributes |
|------|-----------|
| `discord.connect` | — |
| `discord.set_activity` | `discord.state` |

## Metrics (9 total)

### Counters (7)
| Metric | Labels | Purpose |
|--------|--------|---------|
| `position.reports_sent` | — | Successfully sent position reports |
| `position.reports_queued` | — | Reports queued due to failure |
| `position.reports_failed` | — | Reports that failed to send |
| `position.flush_total` | `success` | Flush attempts on flight end |
| `sim.reconnect_attempts` | `adapter`, `success` | Reconnection tracking |
| `sim.staleness_detected` | `adapter` | Stale connection events |
| `api.requests_total` | `method`, `path`, `status` | All outgoing API calls |

### Histograms (2)
| Metric | Labels | Purpose |
|--------|--------|---------|
| `api.request_duration_ms` | `method`, `path` | Latency distribution |
| `position.queue_depth` | — | Queue size at each tick |

## Dependencies Added

- `go.opentelemetry.io/otel`
- `go.opentelemetry.io/otel/sdk`
- `go.opentelemetry.io/otel/exporters/stdout/stdouttrace`
- `go.opentelemetry.io/otel/exporters/stdout/stdoutmetric`
- `go.opentelemetry.io/contrib/bridges/otelslog`

## Future: Swapping to OTLP

Replace stdout exporters in `Init()` with:
```go
import "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
import "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
```
Configure via env vars (`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_HEADERS`). No other files change.
