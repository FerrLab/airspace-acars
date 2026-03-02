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
