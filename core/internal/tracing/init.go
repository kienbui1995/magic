package tracing

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// defaultServiceName is used when OTEL_SERVICE_NAME is unset.
const defaultServiceName = "magic"

// Setup initializes the global OpenTelemetry TracerProvider based on
// standard OTEL_* environment variables. It always installs a
// TextMapPropagator (W3C tracecontext + baggage) so header propagation
// works even without an exporter.
//
// Env vars honored:
//
//	OTEL_EXPORTER_OTLP_ENDPOINT   e.g. "http://localhost:4318" — if unset, a
//	                               no-op tracer is installed.
//	OTEL_EXPORTER_OTLP_PROTOCOL   "http/protobuf" (default) or "grpc".
//	OTEL_SERVICE_NAME             Service name (default: "magic").
//	OTEL_SERVICE_VERSION          Service version (optional).
//	OTEL_TRACES_SAMPLER           "always_on" (default), "always_off",
//	                               "parentbased_always_on",
//	                               "parentbased_traceidratio",
//	                               "traceidratio".
//	OTEL_TRACES_SAMPLER_ARG       Ratio for ratio-based samplers (0.0–1.0).
//	MAGIC_OTEL_STDOUT             "1" to additionally log spans to stdout
//	                               (useful for local debugging).
//
// Setup does not fail if the OTLP endpoint is unreachable; the batch span
// processor buffers spans and retries in the background, so server startup
// is never blocked on the collector.
//
// The returned shutdown function flushes and stops the provider. Callers
// should defer it with a bounded context.
func Setup(ctx context.Context) (func(context.Context) error, error) {
	// Propagator is always installed so worker-to-gateway continuity works
	// even in no-op mode.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		// No-op: leave the global provider alone (otel defaults to noop).
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := newOTLPExporter(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("otel: create OTLP exporter: %w", err)
	}

	res, err := newResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("otel: build resource: %w", err)
	}

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
			sdktrace.WithMaxQueueSize(2048),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(newSampler()),
	}

	if os.Getenv("MAGIC_OTEL_STDOUT") == "1" {
		if stdoutExp, err := stdouttrace.New(stdouttrace.WithPrettyPrint()); err == nil {
			opts = append(opts, sdktrace.WithBatcher(stdoutExp))
		}
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

func newOTLPExporter(ctx context.Context, endpoint string) (sdktrace.SpanExporter, error) {
	protocol := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")))
	if protocol == "" {
		protocol = "http/protobuf"
	}
	// Strip scheme/port handling is done by the SDK when given a URL via
	// env; we pass the endpoint explicitly to keep the surface minimal.
	endpoint = strings.TrimRight(endpoint, "/")
	switch protocol {
	case "grpc":
		target := stripScheme(endpoint)
		return otlptrace.New(ctx, otlptracegrpc.NewClient(
			otlptracegrpc.WithEndpoint(target),
			otlptracegrpc.WithInsecure(), // TLS handled via OTEL_EXPORTER_OTLP_CERTIFICATE etc.
		))
	default: // http/protobuf
		target, insecure := parseHTTPEndpoint(endpoint)
		clientOpts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(target)}
		if insecure {
			clientOpts = append(clientOpts, otlptracehttp.WithInsecure())
		}
		return otlptrace.New(ctx, otlptracehttp.NewClient(clientOpts...))
	}
}

func stripScheme(endpoint string) string {
	for _, prefix := range []string{"http://", "https://"} {
		if strings.HasPrefix(endpoint, prefix) {
			return endpoint[len(prefix):]
		}
	}
	return endpoint
}

// parseHTTPEndpoint returns the host:port portion and whether the connection
// should use plaintext (true when the scheme is http://).
func parseHTTPEndpoint(endpoint string) (host string, insecure bool) {
	switch {
	case strings.HasPrefix(endpoint, "http://"):
		return endpoint[len("http://"):], true
	case strings.HasPrefix(endpoint, "https://"):
		return endpoint[len("https://"):], false
	default:
		// No scheme — assume insecure (collectors usually live on localhost).
		return endpoint, true
	}
}

func newResource(ctx context.Context) (*resource.Resource, error) {
	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = defaultServiceName
	}
	base := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	)
	if v := os.Getenv("OTEL_SERVICE_VERSION"); v != "" {
		base, _ = resource.Merge(base, resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceVersion(v),
		))
	}
	// Merge with environment-derived resource (OTEL_RESOURCE_ATTRIBUTES) and
	// process/host detectors.
	detected, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithHost(),
	)
	if err != nil {
		return base, nil // fall back to minimal resource
	}
	merged, err := resource.Merge(detected, base)
	if err != nil {
		return base, nil
	}
	return merged, nil
}

func newSampler() sdktrace.Sampler {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER")))
	arg := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
	switch name {
	case "always_off":
		return sdktrace.NeverSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(parseRatio(arg, 1.0))
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(parseRatio(arg, 1.0)))
	case "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "", "always_on":
		return sdktrace.AlwaysSample()
	default:
		return sdktrace.AlwaysSample()
	}
}

func parseRatio(s string, fallback float64) float64 {
	if s == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || f > 1 {
		return fallback
	}
	return f
}
