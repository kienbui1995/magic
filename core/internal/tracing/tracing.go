// Package tracing wraps the OpenTelemetry SDK with a small, MagiC-friendly API.
//
// The public surface (StartSpan, Span.SetAttr, Span.End, InjectHeaders,
// ExtractContext / ExtractFromRequest) is stable and does not leak OTel types
// to callers — this lets us swap backends later without touching call sites.
//
// When Setup has not been called (or OTEL_EXPORTER_OTLP_ENDPOINT is unset),
// the package falls back to a no-op tracer: spans are allocated cheaply but
// nothing is exported, and propagation still works for compatibility.
package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// tracerName is the instrumentation scope used for all MagiC core spans.
const tracerName = "github.com/kienbui1995/magic/core"

// Span wraps an OTel span so callers keep their existing `*Span` API.
type Span struct {
	otel oteltrace.Span
}

// StartSpan starts a new span as a child of any span carried by ctx and
// returns the updated context plus the new span. If OTel is not initialized
// the global provider is a no-op and this costs essentially nothing.
func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	tracer := otel.Tracer(tracerName)
	ctx, s := tracer.Start(ctx, name)
	return ctx, &Span{otel: s}
}

// End finishes the span.
func (s *Span) End() {
	if s == nil || s.otel == nil {
		return
	}
	s.otel.End()
}

// SetAttr sets a span attribute. Accepts string, bool, int/int64, or float64;
// anything else is stringified via fmt.Sprint for safety.
func (s *Span) SetAttr(key string, value any) {
	if s == nil || s.otel == nil {
		return
	}
	switch v := value.(type) {
	case string:
		s.otel.SetAttributes(attribute.String(key, v))
	case bool:
		s.otel.SetAttributes(attribute.Bool(key, v))
	case int:
		s.otel.SetAttributes(attribute.Int(key, v))
	case int64:
		s.otel.SetAttributes(attribute.Int64(key, v))
	case float64:
		s.otel.SetAttributes(attribute.Float64(key, v))
	default:
		s.otel.SetAttributes(attribute.String(key, fmt.Sprint(v)))
	}
}

// SetError records an error on the span and marks its status as Error.
func (s *Span) SetError(err error) {
	if s == nil || s.otel == nil || err == nil {
		return
	}
	s.otel.RecordError(err)
	s.otel.SetAttributes(attribute.String("error", err.Error()))
}

// TraceID returns the current trace ID in hex, or "" if no recording span.
func (s *Span) TraceID() string {
	if s == nil || s.otel == nil {
		return ""
	}
	sc := s.otel.SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

// SpanID returns the current span ID in hex, or "" if no recording span.
func (s *Span) SpanID() string {
	if s == nil || s.otel == nil {
		return ""
	}
	sc := s.otel.SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.SpanID().String()
}

// Traceparent returns the W3C traceparent header for this span, or "" if
// no valid span context is available.
func (s *Span) Traceparent() string {
	if s == nil || s.otel == nil {
		return ""
	}
	sc := s.otel.SpanContext()
	if !sc.IsValid() {
		return ""
	}
	flags := "00"
	if sc.IsSampled() {
		flags = "01"
	}
	return fmt.Sprintf("00-%s-%s-%s", sc.TraceID().String(), sc.SpanID().String(), flags)
}

// InjectHeaders writes W3C Trace Context (traceparent/tracestate) headers —
// plus any other propagators registered with the global OTel provider — into
// the outbound request so downstream workers can continue the trace.
func InjectHeaders(ctx context.Context, req *http.Request) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	// Preserve legacy X-Trace-ID header for pre-OTel workers.
	if sc := oteltrace.SpanContextFromContext(ctx); sc.IsValid() {
		if req.Header.Get("X-Trace-ID") == "" {
			req.Header.Set("X-Trace-ID", sc.TraceID().String())
		}
	}
}

// ExtractContext reads incoming tracing headers from req and returns a
// context whose parent span context is populated. Safe to call even if
// no headers are present.
func ExtractContext(ctx context.Context, req *http.Request) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(req.Header))
}

// ExtractFromRequest is kept for backward compatibility. It returns a child
// context derived from the request's own context with any parent span context
// extracted from standard headers. If only the legacy X-Trace-ID header is
// present it synthesizes a remote span context so child spans inherit it.
func ExtractFromRequest(r *http.Request) context.Context {
	ctx := ExtractContext(r.Context(), r)
	if oteltrace.SpanContextFromContext(ctx).IsValid() {
		return ctx
	}
	if raw := r.Header.Get("X-Trace-ID"); raw != "" {
		if tid := parseTraceID(raw); tid.IsValid() {
			sc := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
				TraceID:    tid,
				SpanID:     newSpanID(),
				TraceFlags: oteltrace.FlagsSampled,
				Remote:     true,
			})
			ctx = oteltrace.ContextWithRemoteSpanContext(ctx, sc)
		}
	}
	return ctx
}

// ParseTraceparent is kept for backward compatibility with earlier versions
// of this package.
func ParseTraceparent(header string) (traceID, spanID string, ok bool) {
	parts := strings.Split(header, "-")
	if len(parts) < 4 || parts[0] != "00" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// NewTraceID generates a random 128-bit trace ID in hex form. Exposed for
// callers that want to stamp task.TraceID before any OTel span is started.
func NewTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewSpanID generates a random 64-bit span ID in hex form.
func NewSpanID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// parseTraceID converts a 32-char hex string to an OTel TraceID. Returns the
// zero value (invalid) on parse failure, which callers must check with
// TraceID.IsValid().
func parseTraceID(raw string) oteltrace.TraceID {
	var zero oteltrace.TraceID
	raw = strings.TrimSpace(raw)
	if len(raw) != 32 {
		// Pad shorter values (e.g. legacy "abc123") deterministically so
		// they still produce a stable valid trace ID.
		if len(raw) == 0 || len(raw) > 32 {
			return zero
		}
		raw = strings.Repeat("0", 32-len(raw)) + raw
	}
	tid, err := oteltrace.TraceIDFromHex(raw)
	if err != nil {
		return zero
	}
	return tid
}

func newSpanID() oteltrace.SpanID {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	var sid oteltrace.SpanID
	copy(sid[:], b)
	return sid
}
