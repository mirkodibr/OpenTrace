package exporter

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opentrace/opentrace-go/internal/wire"
)

// mockCollector records every decoded ingest payload it receives.
type mockCollector struct {
	mu       sync.Mutex
	batches  [][]map[string]interface{}
	requests []*http.Request
	status   atomic.Int32 // response status; default 202
	delay    time.Duration
	gzipped  atomic.Int64 // count of gzip-encoded requests
}

func newMockCollector() *mockCollector {
	m := &mockCollector{}
	m.status.Store(http.StatusAccepted)
	return m
}

func (m *mockCollector) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.delay > 0 {
			time.Sleep(m.delay)
		}
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			m.gzipped.Add(1)
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer gz.Close()
			body = gz
		}
		var payload struct {
			Events []map[string]interface{} `json:"events"`
		}
		if err := json.NewDecoder(body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.batches = append(m.batches, payload.Events)
		m.requests = append(m.requests, r.Clone(context.Background()))
		m.mu.Unlock()
		w.WriteHeader(int(m.status.Load()))
	}
}

func (m *mockCollector) eventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, b := range m.batches {
		n += len(b)
	}
	return n
}

func testEvent(msg string, fields ...wire.Field) *wire.LogEvent {
	e := wire.AcquireEvent()
	e.Level = wire.LevelInfo
	e.Message = msg
	e.Timestamp = time.Now()
	e.Fields = append(e.Fields, fields...)
	return e
}

func testResource() wire.Resource {
	return wire.Resource{
		ServiceName:    "exporter-test",
		ServiceVersion: "1.0.0",
		Environment:    "test",
		Hostname:       "host-1",
		PID:            1234,
	}
}

// startExporter wires an exporter to a flush channel and starts it.
func startExporter(t *testing.T, srvURL string, compress bool) (chan []*wire.LogEvent, *HTTPExporter, *atomic.Int64) {
	t.Helper()
	flushCh := make(chan []*wire.LogEvent, 16)
	dropped := &atomic.Int64{}
	exp := New(Config{
		Endpoint:           srvURL,
		SDKVersion:         "test",
		HTTPTimeout:        5 * time.Second,
		CompressionEnabled: compress,
		FlushCh:            flushCh,
		Resource:           testResource(),
		OnDrop:             func(n int64) { dropped.Add(n) },
	})
	exp.Start(context.Background())
	return flushCh, exp, dropped
}

func TestExporterStartStop(t *testing.T) {
	mock := newMockCollector()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	flushCh, exp, dropped := startExporter(t, srv.URL, false)
	for i := 0; i < 10; i++ {
		flushCh <- []*wire.LogEvent{testEvent("batch event"), testEvent("batch event")}
	}
	close(flushCh) // orderly stop: sole-sender close
	exp.Wait()
	exp.Shutdown()

	if got := mock.eventCount(); got != 20 {
		t.Fatalf("mock received %d events, want 20", got)
	}
	if dropped.Load() != 0 {
		t.Fatalf("dropped = %d, want 0", dropped.Load())
	}
	batches, events, failed := exp.Stats()
	if batches != 10 || events != 20 || failed != 0 {
		t.Fatalf("stats = (%d, %d, %d), want (10, 20, 0)", batches, events, failed)
	}
}

func TestExporterShutdownDrainsQueuedBatches(t *testing.T) {
	mock := newMockCollector()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	// Don't start the exporter yet: queue 5 batches first, then start and
	// immediately close — everything queued must still be delivered.
	flushCh := make(chan []*wire.LogEvent, 16)
	exp := New(Config{
		Endpoint:    srv.URL,
		SDKVersion:  "test",
		HTTPTimeout: 5 * time.Second,
		FlushCh:     flushCh,
		Resource:    testResource(),
	})
	for i := 0; i < 5; i++ {
		flushCh <- []*wire.LogEvent{testEvent("queued")}
	}
	close(flushCh)
	exp.Start(context.Background())
	exp.Wait()

	if got := mock.eventCount(); got != 5 {
		t.Fatalf("mock received %d events, want 5", got)
	}
}

func TestExporterContextCancellation(t *testing.T) {
	mock := newMockCollector()
	mock.delay = 50 * time.Millisecond
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	flushCh := make(chan []*wire.LogEvent, 16)
	exp := New(Config{
		Endpoint:    srv.URL,
		SDKVersion:  "test",
		HTTPTimeout: 5 * time.Second,
		FlushCh:     flushCh,
		Resource:    testResource(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	exp.Start(ctx)

	flushCh <- []*wire.LogEvent{testEvent("racing with cancel")}
	cancel()

	finished := make(chan struct{})
	go func() { exp.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("drain loop did not exit after context cancellation")
	}
}

func TestExporterExportAfterShutdown(t *testing.T) {
	flushCh := make(chan []*wire.LogEvent)
	exp := New(Config{
		Endpoint:    "http://127.0.0.1:0",
		SDKVersion:  "test",
		HTTPTimeout: time.Second,
		FlushCh:     flushCh,
		Resource:    testResource(),
	})
	exp.Shutdown()
	if err := exp.Export(context.Background(), []byte("{}")); err != ErrShutdown {
		t.Fatalf("Export after Shutdown = %v, want ErrShutdown", err)
	}
	// Idempotent double-shutdown must not panic.
	exp.Shutdown()
}

func TestExporterGzipRoundTrip(t *testing.T) {
	mock := newMockCollector()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	flushCh, exp, _ := startExporter(t, srv.URL, true)
	// A batch big enough to cross the 1KB compression threshold.
	batch := make([]*wire.LogEvent, 0, 20)
	for i := 0; i < 20; i++ {
		batch = append(batch, testEvent(
			"a reasonably long message to push the payload over the gzip threshold",
			wire.String("user_id", "user-12345"),
			wire.Int("sequence", i),
		))
	}
	flushCh <- batch
	close(flushCh)
	exp.Wait()

	if mock.gzipped.Load() != 1 {
		t.Fatalf("gzipped requests = %d, want 1", mock.gzipped.Load())
	}
	if got := mock.eventCount(); got != 20 {
		t.Fatalf("mock received %d events, want 20", got)
	}
	// Field integrity through compression.
	mock.mu.Lock()
	first := mock.batches[0][0]
	mock.mu.Unlock()
	attrs, ok := first["log_attributes"].(map[string]interface{})
	if !ok {
		t.Fatalf("log_attributes missing: %v", first)
	}
	if attrs["user_id"] != "user-12345" {
		t.Fatalf("user_id = %v, want user-12345", attrs["user_id"])
	}
}

func TestExporterSmallPayloadNotCompressed(t *testing.T) {
	mock := newMockCollector()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	flushCh, exp, _ := startExporter(t, srv.URL, true)
	flushCh <- []*wire.LogEvent{testEvent("tiny")}
	close(flushCh)
	exp.Wait()

	if mock.gzipped.Load() != 0 {
		t.Fatalf("tiny payload was gzipped; threshold not respected")
	}
	if got := mock.eventCount(); got != 1 {
		t.Fatalf("mock received %d events, want 1", got)
	}
}

func TestExporterServerErrorCountsDropped(t *testing.T) {
	mock := newMockCollector()
	mock.status.Store(http.StatusServiceUnavailable)
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	flushCh, exp, dropped := startExporter(t, srv.URL, false)
	flushCh <- []*wire.LogEvent{testEvent("doomed"), testEvent("doomed")}
	close(flushCh)
	exp.Wait()

	if dropped.Load() != 2 {
		t.Fatalf("dropped = %d, want 2", dropped.Load())
	}
	_, _, failed := exp.Stats()
	if failed != 2 {
		t.Fatalf("failed = %d, want 2", failed)
	}
}

func TestExporterHeadersAndWireFormat(t *testing.T) {
	mock := newMockCollector()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	flushCh := make(chan []*wire.LogEvent, 4)
	exp := New(Config{
		Endpoint:    srv.URL,
		SDKVersion:  "9.9.9",
		HTTPTimeout: 5 * time.Second,
		Headers:     map[string]string{"X-API-Key": "secret-key"},
		FlushCh:     flushCh,
		Resource:    testResource(),
	})
	exp.Start(context.Background())

	flushCh <- []*wire.LogEvent{testEvent("wire check",
		wire.String("trace_id", "0af7651916cd43dd8448eb211c80319c"),
		wire.String("span_id", "b7ad6b7169203331"),
		wire.Duration("elapsed", 1500*time.Millisecond),
		wire.Bool("ok", true),
	)}
	close(flushCh)
	exp.Wait()

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(mock.requests))
	}
	r := mock.requests[0]
	if r.Header.Get("X-SDK-Version") != "9.9.9" {
		t.Errorf("X-SDK-Version = %q", r.Header.Get("X-SDK-Version"))
	}
	if r.Header.Get("User-Agent") != "opentrace-go/9.9.9" {
		t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
	}
	if r.Header.Get("X-API-Key") != "secret-key" {
		t.Errorf("X-API-Key = %q", r.Header.Get("X-API-Key"))
	}
	if r.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
	}

	evt := mock.batches[0][0]
	if evt["service_name"] != "exporter-test" {
		t.Errorf("service_name = %v", evt["service_name"])
	}
	if evt["severity"] != "info" {
		t.Errorf("severity = %v", evt["severity"])
	}
	if evt["body"] != "wire check" {
		t.Errorf("body = %v", evt["body"])
	}
	// trace/span IDs lifted to top level, not left in log_attributes.
	if evt["trace_id"] != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("trace_id = %v", evt["trace_id"])
	}
	if evt["span_id"] != "b7ad6b7169203331" {
		t.Errorf("span_id = %v", evt["span_id"])
	}
	attrs := evt["log_attributes"].(map[string]interface{})
	if _, present := attrs["trace_id"]; present {
		t.Error("trace_id was not lifted out of log_attributes")
	}
	if attrs["elapsed"] != 1500.0 { // duration serialised as float64 ms
		t.Errorf("elapsed = %v, want 1500.0", attrs["elapsed"])
	}
	if attrs["ok"] != true {
		t.Errorf("ok = %v", attrs["ok"])
	}
	ra := evt["resource_attributes"].(map[string]interface{})
	if ra["service.name"] != "exporter-test" || ra["host.name"] != "host-1" {
		t.Errorf("resource_attributes = %v", ra)
	}
	// Timestamp parses as RFC3339 (collector validation requirement).
	if _, err := time.Parse(time.RFC3339, evt["timestamp"].(string)); err != nil {
		t.Errorf("timestamp %v not RFC3339: %v", evt["timestamp"], err)
	}
}

func TestExporterConcurrentExport(t *testing.T) {
	mock := newMockCollector()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	flushCh := make(chan []*wire.LogEvent)
	exp := New(Config{
		Endpoint:    srv.URL,
		SDKVersion:  "test",
		HTTPTimeout: 5 * time.Second,
		FlushCh:     flushCh,
		Resource:    testResource(),
	})
	// Export (the payload path) must be safe for concurrent callers even
	// though the drain loop is single-goroutine — the retry layer and
	// Fatal-flush paths may overlap.
	payload := []byte(`{"events":[]}`)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := exp.Export(context.Background(), payload); err != nil {
				t.Errorf("concurrent Export: %v", err)
			}
		}()
	}
	wg.Wait()
}

// TestSerializeAnyFallback ensures unmarshalable Any values degrade to
// strings instead of poisoning the batch.
func TestSerializeAnyFallback(t *testing.T) {
	var buf bytes.Buffer
	ch := make(chan int) // json.Marshal fails on channels
	e := testEvent("poison", wire.Any("bad", ch), wire.Any("good", 42))
	err := serialize(&buf, []*wire.LogEvent{e}, testResource())
	wire.ReleaseEvent(e)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}
	var payload struct {
		Events []map[string]interface{} `json:"events"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	attrs := payload.Events[0]["log_attributes"].(map[string]interface{})
	if _, isString := attrs["bad"].(string); !isString {
		t.Errorf("unmarshalable Any value did not degrade to string: %T", attrs["bad"])
	}
	if attrs["good"] != 42.0 {
		t.Errorf("good = %v, want 42", attrs["good"])
	}
}
