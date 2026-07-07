package opentrace

import (
	"context"
	"os"
)

// resourceInfo holds host/process attributes captured once at SDK init.
// These are attached to every exported event without repeating the syscalls
// on the hot path.
type resourceInfo struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Hostname       string
	PID            int
}

func captureResource(cfg *Config) resourceInfo {
	hostname, _ := os.Hostname()
	return resourceInfo{
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		Environment:    cfg.Environment,
		Hostname:       hostname,
		PID:            os.Getpid(),
	}
}

// traceContextKey is the SDK's own context key for trace correlation IDs.
//
// The OpenTelemetry SDK stores its span context under a key type that is
// unexported from otel/trace, so it is impossible to read it without
// importing that module. Rather than ship dead code that pretends to,
// the SDK defines its own carrier: applications (or a thin OTel bridge)
// call ContextWithTrace to make IDs visible to WithContext.
type traceContextKey struct{}

type traceContext struct {
	traceID string
	spanID  string
}

// ContextWithTrace returns a copy of ctx carrying the given trace and span
// IDs. Loggers derived via WithContext from the returned context attach the
// IDs as trace_id / span_id fields on every event.
//
// Applications using OpenTelemetry can bridge in one line:
//
//	sc := trace.SpanContextFromContext(ctx)
//	ctx = opentrace.ContextWithTrace(ctx, sc.TraceID().String(), sc.SpanID().String())
func ContextWithTrace(ctx context.Context, traceID, spanID string) context.Context {
	return context.WithValue(ctx, traceContextKey{}, traceContext{traceID: traceID, spanID: spanID})
}

// extractTraceContext reads IDs previously stored by ContextWithTrace.
// Returns empty strings when the context carries no trace information.
func extractTraceContext(ctx context.Context) (traceID, spanID string) {
	if tc, ok := ctx.Value(traceContextKey{}).(traceContext); ok {
		return tc.traceID, tc.spanID
	}
	return "", ""
}
