// Package exporter implements Layer 3 of the SDK pipeline (ADR-005): a
// single background goroutine that drains batches from the flush channel,
// serialises them to the collector wire format, and transmits them over
// HTTP with optional gzip compression.
//
// # Concurrency and shutdown races (documented invariants)
//
//  1. Double-close: Shutdown is guarded by sync.Once — calling it twice is
//     safe and the second call waits for the first to finish.
//  2. Export after Shutdown: once the shutdown flag is set, Export returns
//     ErrShutdown immediately without touching the network.
//  3. flushCh send/drain at the shutdown boundary: the batcher is the sole
//     sender on flushCh and closes it after its final sweep. The drain loop
//     exits only when the channel is closed AND empty (range semantics), so
//     every batch handed to the channel before close is exported or
//     explicitly dropped — never silently lost.
//  4. Context cancellation during an active export: the HTTP request is
//     built with the run context; cancelling it aborts the in-flight
//     request, the batch is counted as dropped, and the loop continues
//     draining so the channel never blocks the batcher.
package exporter

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opentrace/opentrace-go/internal/wire"
)

// ErrShutdown is returned by Export when the exporter has been shut down.
var ErrShutdown = errors.New("opentrace: exporter is shut down")

// HTTPError describes a non-2xx collector response. It satisfies the
// status-carrying interface the retry layer classifies on, without the
// retry package needing to import this one.
type HTTPError struct {
	Status     int
	RetryAfter time.Duration // parsed from the Retry-After header, 0 if absent
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("opentrace: collector returned HTTP %d", e.Status)
}

// HTTPStatus implements the retry layer's classification interface.
func (e *HTTPError) HTTPStatus() int { return e.Status }

// RetryAfterHint implements the retry layer's classification interface.
func (e *HTTPError) RetryAfterHint() time.Duration { return e.RetryAfter }

// Config parameterises an HTTPExporter.
type Config struct {
	Endpoint             string // collector base URL, e.g. http://localhost:8080
	Headers              map[string]string
	SDKVersion           string
	HTTPTimeout          time.Duration
	CompressionEnabled   bool
	CompressionThreshold int  // gzip payloads larger than this many bytes (default 1024)
	Debug                bool // emit SDK-internal diagnostics to stderr

	FlushCh  <-chan []*wire.LogEvent
	Resource wire.Resource
	// OnDrop is invoked with the number of events lost when an export
	// fails permanently or is skipped.
	OnDrop func(int64)
	// Send wraps the final transmission. Defaults to plain HTTP POST; the
	// retry layer (Day 26) decorates it with backoff and circuit breaking.
	Send func(ctx context.Context, fn func(context.Context) error) error
}

// HTTPExporter drains the flush channel and POSTs batches to the collector.
type HTTPExporter struct {
	cfg      Config
	client   *http.Client
	url      string
	shutdown atomic.Bool
	once     sync.Once
	done     chan struct{}

	// Exported counters for observability and tests.
	batchesSent  atomic.Int64
	eventsSent   atomic.Int64
	eventsFailed atomic.Int64
}

// bufPool recycles serialisation and compression buffers across batches.
var bufPool = sync.Pool{
	New: func() interface{} { return new(bytes.Buffer) },
}

// New constructs an HTTPExporter. Call Start exactly once.
func New(cfg Config) *HTTPExporter {
	if cfg.OnDrop == nil {
		cfg.OnDrop = func(int64) {}
	}
	if cfg.CompressionThreshold <= 0 {
		cfg.CompressionThreshold = 1024
	}
	if cfg.Send == nil {
		cfg.Send = func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}
	}
	return &HTTPExporter{
		cfg: cfg,
		url: cfg.Endpoint + "/api/v1/logs",
		client: &http.Client{
			Timeout: cfg.HTTPTimeout,
			Transport: &http.Transport{
				MaxIdleConns:    10,
				IdleConnTimeout: 90 * time.Second,
				// We gzip request bodies ourselves; response compression
				// is irrelevant for the tiny 202 acknowledgements.
				DisableCompression: true,
			},
		},
		done: make(chan struct{}),
	}
}

// Start spawns the drain-loop goroutine. ctx cancellation is the emergency
// stop; the orderly path is the batcher closing the flush channel.
func (e *HTTPExporter) Start(ctx context.Context) {
	go func() {
		defer close(e.done)
		for {
			select {
			case batch, ok := <-e.cfg.FlushCh:
				if !ok {
					return // orderly shutdown: channel closed and drained
				}
				e.exportBatch(ctx, batch)
			case <-ctx.Done():
				e.drainAndClose(ctx)
				return
			}
		}
	}()
}

// Wait blocks until the drain loop has exited.
func (e *HTTPExporter) Wait() { <-e.done }

// Shutdown marks the exporter as stopped and closes idle connections. It is
// idempotent. The caller is responsible for first stopping the batcher so
// the flush channel closes and the drain loop exits; Wait() observes that.
func (e *HTTPExporter) Shutdown() {
	e.once.Do(func() {
		e.shutdown.Store(true)
		e.client.CloseIdleConnections()
	})
}

// Stats returns (batches sent, events sent, events failed).
func (e *HTTPExporter) Stats() (int64, int64, int64) {
	return e.batchesSent.Load(), e.eventsSent.Load(), e.eventsFailed.Load()
}

// drainAndClose empties any batches still queued in the flush channel after
// an emergency stop, bounded by a 5-second grace window.
func (e *HTTPExporter) drainAndClose(ctx context.Context) {
	grace, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	drained := 0
	for {
		select {
		case batch, ok := <-e.cfg.FlushCh:
			if !ok {
				e.debugf("drain complete: %d batches", drained)
				return
			}
			e.exportBatch(grace, batch)
			drained++
		default:
			e.debugf("drain complete: %d batches", drained)
			return
		}
	}
}

// exportBatch serialises, releases the events (ADR-005 D2: before any
// network I/O — retries operate on bytes only), and transmits.
func (e *HTTPExporter) exportBatch(ctx context.Context, batch []*wire.LogEvent) {
	n := int64(len(batch))
	if n == 0 {
		return
	}

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	err := serialize(buf, batch, e.cfg.Resource)
	for _, evt := range batch {
		wire.ReleaseEvent(evt)
	}
	if err != nil {
		e.eventsFailed.Add(n)
		e.cfg.OnDrop(n)
		e.debugf("serialisation failed, %d events dropped: %v", n, err)
		return
	}

	start := time.Now()
	if err := e.Export(ctx, buf.Bytes()); err != nil {
		e.eventsFailed.Add(n)
		e.cfg.OnDrop(n)
		e.debugf("export failed after %s, %d events dropped: %v", time.Since(start), n, err)
		return
	}
	e.batchesSent.Add(1)
	e.eventsSent.Add(n)
	e.debugf("exported %d events (%dB) in %s", n, buf.Len(), time.Since(start))
}

// Export transmits an already-serialised payload through the configured
// Send wrapper (identity by default, retry+breaker once Day 26 lands).
func (e *HTTPExporter) Export(ctx context.Context, payload []byte) error {
	if e.shutdown.Load() {
		return ErrShutdown
	}
	body := payload
	compressed := false
	if e.cfg.CompressionEnabled && len(payload) > e.cfg.CompressionThreshold {
		gz := bufPool.Get().(*bytes.Buffer)
		gz.Reset()
		defer bufPool.Put(gz)
		w := gzip.NewWriter(gz)
		if _, err := w.Write(payload); err == nil && w.Close() == nil {
			body = gz.Bytes()
			compressed = true
		}
	}
	return e.cfg.Send(ctx, func(ctx context.Context) error {
		return e.post(ctx, body, compressed)
	})
}

// post performs one HTTP POST attempt.
func (e *HTTPExporter) post(ctx context.Context, body []byte, compressed bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("opentrace: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if compressed {
		req.Header.Set("Content-Encoding", "gzip")
	}
	req.Header.Set("X-SDK-Version", e.cfg.SDKVersion)
	req.Header.Set("User-Agent", "opentrace-go/"+e.cfg.SDKVersion)
	for k, v := range e.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("opentrace: posting batch: %w", err)
	}
	// Always drain and close the body so the connection returns to the pool.
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{
			Status:     resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}
	return nil
}

// parseRetryAfter handles the delta-seconds form of the Retry-After header.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	var secs int
	if _, err := fmt.Sscanf(v, "%d", &secs); err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

func (e *HTTPExporter) debugf(format string, args ...interface{}) {
	if e.cfg.Debug {
		fmt.Fprintf(os.Stderr, "[opentrace-sdk] "+format+"\n", args...)
	}
}
