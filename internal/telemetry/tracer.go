// Package telemetry provides OpenTelemetry tracer initialization and span
// helpers. The default exporter writes JSONL to a file; emission is opt-in
// via the trace path argument. When no trace path is configured the package
// installs a no-op tracer provider so StartSpan/EndSpan calls remain valid
// and cheap.
package telemetry

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// EnvTraceFile is the environment variable that opts a process into tracing
// by pointing at a writable JSONL trace file.
const EnvTraceFile = "GDRIVE_TRACE_FILE"

// noopShutdown is returned when tracing is disabled. Callers can defer it
// without conditional branches.
func noopShutdown(context.Context) error { return nil }

// InitTracer initializes the global tracer provider. If tracePath is empty,
// tracing is disabled (returns a no-op shutdown). Otherwise spans are
// exported as JSON lines to the given file. The returned function flushes
// and shuts down the provider; callers should defer it from main.
func InitTracer(serviceName, tracePath string) (func(context.Context) error, error) {
	if tracePath == "" {
		return noopShutdown, nil
	}

	f, err := os.OpenFile(tracePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open trace file %s: %w", tracePath, err)
	}

	exporter, err := stdouttrace.New(stdouttrace.WithWriter(f))
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
	)
	otel.SetTracerProvider(tp)

	shutdown := func(ctx context.Context) error {
		shutdownErr := tp.Shutdown(ctx)
		closeErr := f.Close()
		if shutdownErr != nil {
			return fmt.Errorf("tracer shutdown: %w", shutdownErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close trace file: %w", closeErr)
		}
		return nil
	}
	return shutdown, nil
}

// InitFromEnv is a convenience wrapper that reads the trace file path from
// EnvTraceFile and calls InitTracer.
func InitFromEnv(serviceName string) (func(context.Context) error, error) {
	return InitTracer(serviceName, os.Getenv(EnvTraceFile))
}

// Discard returns an io.Writer that drops all data; provided for tests.
func Discard() io.Writer { return io.Discard }
