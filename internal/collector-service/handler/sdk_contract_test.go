package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	opentrace "github.com/opentrace/opentrace-go"
	"github.com/opentrace/opentrace/pkg/schema"
)

// TestSDKWireFormatContract is the golden contract test between the Go SDK
// serialiser and the collector's ingest schema (ADR-006): a payload emitted
// by the real SDK must decode losslessly into schema.IngestLogsRequest and
// pass the collector's validation rules.
func TestSDKWireFormatContract(t *testing.T) {
	var (
		mu       sync.Mutex
		captured []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = body
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	logger, err := opentrace.New(
		opentrace.WithCollectorEndpoint(srv.URL),
		opentrace.WithServiceName("contract-test"),
		opentrace.WithServiceVersion("1.2.3"),
		opentrace.WithEnvironment("test"),
		opentrace.WithCompression(false), // capture plain JSON
	)
	if err != nil {
		t.Fatalf("SDK New: %v", err)
	}

	traced := logger.WithContext(opentrace.ContextWithTrace(context.Background(),
		"0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331"))
	traced.Info("contract event",
		opentrace.String("user_id", "u_42"),
		opentrace.Int("item_count", 3),
		opentrace.Float64("total", 99.95),
		opentrace.Bool("cache_hit", true),
		opentrace.Duration("elapsed", 1500*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := logger.Shutdown(ctx); err != nil {
		t.Fatalf("SDK Shutdown: %v", err)
	}

	mu.Lock()
	payload := captured
	mu.Unlock()
	if len(payload) == 0 {
		t.Fatal("no payload captured from the SDK")
	}

	// 1. Decodes into the canonical schema type.
	var req schema.IngestLogsRequest
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields() // the SDK must not emit fields the schema doesn't know
	if err := dec.Decode(&req); err != nil {
		t.Fatalf("SDK payload does not decode into schema.IngestLogsRequest: %v\npayload: %s", err, payload)
	}
	if len(req.Events) != 1 {
		t.Fatalf("decoded %d events, want 1", len(req.Events))
	}

	// 2. Passes the collector's own validation.
	if violations := validateIngestRequest(&req, 1000); len(violations) > 0 {
		t.Fatalf("SDK payload fails collector validation: %+v", violations)
	}

	// 3. Field-level round-trip integrity.
	e := req.Events[0]
	if e.ServiceName != "contract-test" {
		t.Errorf("service_name = %q", e.ServiceName)
	}
	if e.Severity != schema.LogLevelInfo {
		t.Errorf("severity = %q, want info", e.Severity)
	}
	if e.Body != "contract event" {
		t.Errorf("body = %q", e.Body)
	}
	if e.TraceID != "0af7651916cd43dd8448eb211c80319c" || e.SpanID != "b7ad6b7169203331" {
		t.Errorf("trace correlation lost: trace_id=%q span_id=%q", e.TraceID, e.SpanID)
	}
	if e.Timestamp.IsZero() || time.Since(e.Timestamp) > time.Minute {
		t.Errorf("timestamp = %v, not recent", e.Timestamp)
	}
	if got := e.LogAttributes["user_id"]; got != "u_42" {
		t.Errorf("log_attributes.user_id = %v", got)
	}
	if got := e.LogAttributes["item_count"]; got != 3.0 {
		t.Errorf("log_attributes.item_count = %v", got)
	}
	if got := e.LogAttributes["elapsed"]; got != 1500.0 {
		t.Errorf("log_attributes.elapsed = %v, want 1500 (ms)", got)
	}
	if got := e.ResourceAttributes["service.name"]; got != "contract-test" {
		t.Errorf("resource_attributes[service.name] = %v", got)
	}
	if got := e.ResourceAttributes["service.version"]; got != "1.2.3" {
		t.Errorf("resource_attributes[service.version] = %v", got)
	}
}
