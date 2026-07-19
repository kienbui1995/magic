package tracing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// withInMemoryTracer installs an in-memory span exporter for the duration of
// the test and returns the recorder so tests can inspect emitted spans.
func withInMemoryTracer(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	prev := otel.GetTracerProvider()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(rec),
	)
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prev)
	})
	// Propagator is idempotent — Setup would install it, mirror that here.
	_, _ = Setup(context.Background())
	return rec
}

func TestStartSpan_NoopWhenUnset(t *testing.T) {
	// No provider installed — StartSpan must still return a usable span and
	// must not panic.
	ctx, span := StartSpan(context.Background(), "noop-op")
	if ctx == nil || span == nil {
		t.Fatal("StartSpan must return non-nil ctx and span even without setup")
	}
	span.SetAttr("k", "v")
	span.SetError(errors.New("boom"))
	span.End()
}

func TestSetup_NoEndpointIsNoop(t *testing.T) {
	os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	shutdown, err := Setup(context.Background())
	if err != nil {
		t.Fatalf("Setup unexpectedly failed: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Setup must return non-nil shutdown fn")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown returned error: %v", err)
	}
}

func TestStartSpan_CapturesAttrsAndParent(t *testing.T) {
	rec := withInMemoryTracer(t)

	ctx, parent := StartSpan(context.Background(), "parent-op")
	parent.SetAttr("task.id", "abc")
	parent.SetAttr("worker.count", 3)
	parent.SetAttr("retry", true)

	_, child := StartSpan(ctx, "child-op")
	child.End()
	parent.End()

	spans := rec.Ended()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	// Children end first — first span is the child.
	if spans[0].Parent().SpanID() != spans[1].SpanContext().SpanID() {
		t.Error("child span parent link does not match parent span ID")
	}
	if spans[0].SpanContext().TraceID() != spans[1].SpanContext().TraceID() {
		t.Error("child must inherit parent trace ID")
	}

	// Attribute check on parent.
	foundTaskID := false
	for _, kv := range spans[1].Attributes() {
		if string(kv.Key) == "task.id" && kv.Value.AsString() == "abc" {
			foundTaskID = true
		}
	}
	if !foundTaskID {
		t.Error("parent span missing task.id attribute")
	}
}

func TestInjectExtract_RoundTrip(t *testing.T) {
	withInMemoryTracer(t)

	ctx, span := StartSpan(context.Background(), "origin")
	defer span.End()
	originTrace := span.TraceID()
	if originTrace == "" {
		t.Fatal("origin span has no trace ID")
	}

	req := httptest.NewRequest("GET", "/", nil)
	InjectHeaders(ctx, req)

	if req.Header.Get("Traceparent") == "" {
		t.Error("traceparent header not injected")
	}
	if req.Header.Get("X-Trace-ID") == "" {
		t.Error("legacy X-Trace-ID not set")
	}

	extracted := ExtractContext(context.Background(), req)
	sc := oteltrace.SpanContextFromContext(extracted)
	if !sc.IsValid() {
		t.Fatal("extracted context has no valid span context")
	}
	if sc.TraceID().String() != originTrace {
		t.Errorf("trace ID not preserved: got %s want %s", sc.TraceID().String(), originTrace)
	}
}

func TestExtractFromRequest_LegacyXTraceID(t *testing.T) {
	withInMemoryTracer(t)
	req, _ := http.NewRequest("GET", "/", nil)
	// 32 hex chars — must round-trip via OTel TraceID.
	req.Header.Set("X-Trace-ID", "0af7651916cd43dd8448eb211c80319c")
	ctx := ExtractFromRequest(req)
	sc := oteltrace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		t.Fatal("expected valid remote span context from X-Trace-ID")
	}
	if sc.TraceID().String() != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("trace ID mismatch: %s", sc.TraceID().String())
	}
}

func TestParseTraceparent_Legacy(t *testing.T) {
	tid, sid, ok := ParseTraceparent("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	if !ok {
		t.Fatal("failed to parse valid traceparent")
	}
	if tid != "0af7651916cd43dd8448eb211c80319c" || sid != "b7ad6b7169203331" {
		t.Errorf("ids mismatch: %s / %s", tid, sid)
	}
}
