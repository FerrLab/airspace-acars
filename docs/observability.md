# Error reporting

The ACARS reports its own failures to [Sentry](https://sentry.io). This replaced
an OpenTelemetry setup that wrote traces and metrics to JSONL files next to the
executable: those files only helped when a pilot thought to send them, which in
practice meant a crash mid-flight left no trace at all.

Both halves of the application report into one project and are told apart by a
`side` tag: `backend` for Go, `frontend` for the webview.

## What is reported

| | |
|---|---|
| **Errors** | Anything passed to `observability.Capture`, any span closed with `Fail`, and every `slog.Error` — the log bridge means a failure that never reaches a span is still visible |
| **Panics** | Every long-lived goroutine defers `observability.Recover()`, so a panic in the data stream or the position reporter is reported instead of taking the window down silently |
| **UI crashes** | A React error boundary catches a render crash, shows the pilot a reload button, and reports it. Unhandled promise rejections — usually a failed Wails call — are reported too |
| **Performance** | A 5% sample of spans. A long-haul flight would otherwise ship a transaction every second of it |

## What is not

Nothing that identifies the machine or the person flying it:

- `SendDefaultPII` is off and `ServerName` is fixed to `desktop`, so the account
  name and the machine name never leave.
- The only identifier attached is the pilot id the network itself issues, set by
  `SetPilot` when the pilot signs in. The name and email address the same API
  response carries are not sent.
- Every free-form string is scrubbed on the way out — messages, exception
  values, breadcrumbs, span data, tags. Bearer tokens, JSON Web Tokens, device
  codes, secrets in query strings and anything under the pilot's profile
  directory are replaced with `[redacted]`, the last of which becomes `~`.

Scrubbing runs in `BeforeSend` *and* in `BeforeBreadcrumb`, so a secret is never
even held in memory waiting for an error to carry it out. `observability/scrub_test.go`
is the specification: it asserts the secrets go and, just as importantly, that
ordinary diagnostics survive — redaction that eats the diagnosis is redaction
nobody will trust.

## Counters and gauges

Sentry has no home for a free-standing counter, so the OpenTelemetry ones became
two things:

- **A running total** attached to every report as the `counters` context. A
  report then says what the application had been doing, not only what finally
  broke: *"the position reporter failed 12 times before this crash"*.
- **A breadcrumb**, rate-limited to one per counter every 30 seconds and
  carrying however many occurrences it stands for. Without the limit the
  position reporter — which counts every report it ships — would push
  everything else out of the 100-slot ring buffer.

Histograms became `observability.Gauge`: the latest value only, no breadcrumb,
attached to reports alongside the counters. A queue depth sampled every tick is
noise as a breadcrumb but exactly the right context on an error.

```go
observability.Count("sim.reconnect_attempts", "adapter", adapterName)
observability.Add("position.reports_sent", int64(sent))
observability.Gauge("position.outbox_depth", float64(count))
```

## Instrumenting

```go
ctx, span := observability.Start(ctx, "sim.connect", "sim.type", simType)
defer span.Finish()

span.Set("sim.adapter", connector.Name())

if err := connector.Connect(); err != nil {
    span.Fail(err)   // marks the span failed and reports the error
    return err
}
```

Keys and values are slog-shaped pairs throughout, so `Start`, `Set`, `Count`,
`Capture` and `Note` all read the same way as the `slog` calls beside them.

Every one of these is a no-op when no DSN is configured, so nothing needs a nil
check and a build without reporting behaves exactly as it did before.

## Turning it on

The DSN is injected at build time and is empty in a plain `go build`. A fork, a
local build and `wails3 dev` therefore report nothing at all.

| | |
|---|---|
| Release builds | `SENTRY_DSN` repository secret → `-X airspace-acars/observability.DSN=…` for Go and `VITE_SENTRY_DSN` for the frontend |
| Local build | `SENTRY_DSN=… wails3 task windows:build` |
| At runtime | `AIRSPACE_SENTRY_DSN=…` overrides whatever was built in — the way to point a build at your own project |
| Trace sampling | `AIRSPACE_SENTRY_TRACES=0.5`, between 0 and 1, default 0.05 |

Setting `AIRSPACE_SENTRY_DSN=` to empty switches reporting off in a release
build.

## The log file

`stdout.log`, next to the executable, is unchanged. The Settings window reads it
back through `TailLogs`, and it is the only record a pilot can send by hand when
reporting is switched off. Errors written to it are also reported, which is the
single cheapest source of coverage in the application — every `slog.Error` that
already existed became a report without anyone editing the call site.
