package opentrace

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// collectorMock is a minimal stand-in for the OpenTrace collector: it
// decodes (optionally gzipped) ingest payloads and counts events.
type collectorMock struct {
	mu     sync.Mutex
	events []map[string]interface{}
}

func (c *collectorMock) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
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
		c.mu.Lock()
		c.events = append(c.events, payload.Events...)
		c.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}
}

func (c *collectorMock) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

// TestPipelineEndToEnd exercises the full wired SDK: New → Info×N →
// Shutdown, asserting every accepted event reaches the mock collector.
func TestPipelineEndToEnd(t *testing.T) {
	mock := &collectorMock{}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	logger, err := New(
		WithCollectorEndpoint(srv.URL),
		WithServiceName("pipeline-test"),
		WithBatchSize(25),
		WithBatchInterval(100*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const total = 200
	for i := 0; i < total; i++ {
		logger.Info("pipeline event", Int("seq", i), String("source", "e2e"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := logger.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	received := mock.count()
	droppedN := logger.DroppedCount()
	if int64(received)+droppedN != total {
		t.Fatalf("received %d + dropped %d != sent %d", received, droppedN, total)
	}
	if received == 0 {
		t.Fatal("no events reached the collector")
	}
	if droppedN != 0 {
		t.Logf("note: %d events dropped (accounted)", droppedN)
	}

	// Logging after shutdown drops without panic.
	logger.Info("late event")
	if logger.DroppedCount() != droppedN+1 {
		t.Fatalf("post-shutdown event not counted as dropped")
	}
}

// TestPipelineIntervalFlush verifies events flow without reaching the size
// trigger (interval-driven flush) while the logger stays alive.
func TestPipelineIntervalFlush(t *testing.T) {
	mock := &collectorMock{}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	logger, err := New(
		WithCollectorEndpoint(srv.URL),
		WithServiceName("interval-test"),
		WithBatchSize(1000), // never reached
		WithBatchInterval(100*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer logger.Shutdown(context.Background())

	logger.Info("interval event")
	deadline := time.Now().Add(5 * time.Second)
	for mock.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if mock.count() != 1 {
		t.Fatalf("interval flush did not deliver the event within 5s")
	}
}

// TestPipelineChildLoggerSharesShutdown verifies With() children observe
// the parent's shutdown and share the dropped counter.
func TestPipelineChildLoggerSharesShutdown(t *testing.T) {
	mock := &collectorMock{}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	logger, err := New(
		WithCollectorEndpoint(srv.URL),
		WithServiceName("child-test"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	child := logger.With(String("component", "worker"))

	if err := logger.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	child.Info("after parent shutdown")
	if logger.DroppedCount() != 1 {
		t.Fatalf("child logger did not share the closed flag / dropped counter")
	}
}
