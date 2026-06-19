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

// otelTraceContextKey is the interface type used by the OpenTelemetry Go SDK
// to store span context in a context.Context. We use interface assertion
// rather than importing the full OTel SDK to keep the dependency footprint
// minimal for non-OTel users.
type otelSpanContextKey struct{}

// extractTraceContext extracts trace_id and span_id from ctx without importing
// the OpenTelemetry SDK. Returns empty strings when no span is present.
//
// Compatible with github.com/open-telemetry/opentelemetry-go span context
// stored under the standard context key.
func extractTraceContext(ctx context.Context) (traceID, spanID string) {
	// Type-assert against the OTel SpanContext interface.
	// This compiles even when the OTel SDK is not in the dependency graph.
	type spanContextCarrier interface {
		TraceID() [16]byte
		SpanID() [8]byte
		IsValid() bool
	}
	if sc, ok := ctx.Value(otelSpanContextKey{}).(spanContextCarrier); ok && sc.IsValid() {
		tid := sc.TraceID()
		sid := sc.SpanID()
		return hexEncodeBytes(tid[:]), hexEncodeBytes(sid[:])
	}
	return "", ""
}

// hexEncodeBytes encodes b as a lowercase hex string without importing
// encoding/hex to avoid unnecessary allocations in the common case where
// trace context is absent.
func hexEncodeBytes(b []byte) string {
	const hextable = "0123456789abcdef"
	dst := make([]byte, len(b)*2)
	for i, v := range b {
		dst[i*2] = hextable[v>>4]
		dst[i*2+1] = hextable[v&0x0f]
	}
	return string(dst)
}
