package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracerName identifies spans emitted by this binary.
const tracerName = "gdrive"

// StartSpan creates a span on the global tracer with the supplied attributes
// and returns the derived context plus the span. Callers must call EndSpan
// (typically via defer) so the span's status is recorded and flushed.
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	tracer := otel.Tracer(tracerName)
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// EndSpan records an error status on span if err is non-nil, marks success
// otherwise, then ends the span. Sensitive payloads (tokens, credentials,
// LLM prompts/responses, PII) MUST NOT be passed as span attributes.
func EndSpan(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}
