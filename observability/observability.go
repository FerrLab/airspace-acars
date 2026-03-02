package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

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

// exeDir returns the directory containing the running executable.
// All observability files are written relative to this directory so
// the app works correctly regardless of the current working directory
// (e.g. when launched from a shortcut or Start Menu on Windows).
func exeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// Init sets up OpenTelemetry tracing, metrics, and structured log output.
// It returns a shutdown function that must be called on app exit (typically via defer).
// All output files (traces.jsonl, metrics.jsonl, stdout.log) are written
// next to the executable.
func Init(serviceName, version string) (shutdown func(context.Context) error, err error) {
	dir, err := exeDir()
	if err != nil {
		return nil, err
	}

	res, err := resource.New(context.Background(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return nil, err
	}

	// --- Log file ---
	logFile, err := os.OpenFile(filepath.Join(dir, "stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(logFile, nil)))

	// --- Tracing ---
	traceFile, err := os.OpenFile(filepath.Join(dir, "traces.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logFile.Close()
		return nil, err
	}
	traceExp, err := stdouttrace.New(stdouttrace.WithWriter(traceFile))
	if err != nil {
		closeAll(logFile, traceFile)
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// --- Metrics ---
	metricFile, err := os.OpenFile(filepath.Join(dir, "metrics.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		closeAll(logFile, traceFile)
		return nil, err
	}
	metricExp, err := stdoutmetric.New(stdoutmetric.WithWriter(metricFile))
	if err != nil {
		closeAll(logFile, traceFile, metricFile)
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(30*time.Second))),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	shutdown = func(ctx context.Context) error {
		var firstErr error
		if e := tp.Shutdown(ctx); e != nil && firstErr == nil {
			firstErr = e
		}
		if e := mp.Shutdown(ctx); e != nil && firstErr == nil {
			firstErr = e
		}
		closeAll(traceFile, metricFile, logFile)
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
