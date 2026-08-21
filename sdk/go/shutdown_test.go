package opentrace

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// TestMain verifies no goroutine started anywhere in this package's test
// suite is still running once all tests complete — the ground-truth check
// for the pipeline's goroutine lifecycle claims (ADR-005, Day 33).
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// http.DefaultTransport's idle-connection reaper is a background
		// goroutine owned by the Go runtime/stdlib, not the SDK; it is not
		// a leak this suite is responsible for.
		goleak.IgnoreTopFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).writeLoop"),
	)
}

// countingMock counts received events and can simulate per-response delay.
type countingMock struct {
	mu       sync.Mutex
	received int
	delay    time.Duration
}

// handler mirrors the real collector: it must decompress gzip bodies
// before decoding, since the SDK compresses batches over ~1KB by default
// (WithCompression(true)). A mock that skips this, as an earlier version
// of this test did, silently turns every large batch into a 400 and
// every event into a false "dropped" — caught by running these tests
// against a real HTTP round trip rather than only unit-testing in
// isolation.
func (m *countingMock) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.delay > 0 {
			time.Sleep(m.delay)
		}
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
			Events []json.RawMessage `json:"events"`
		}
		if err := json.NewDecoder(body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.received += len(payload.Events)
		m.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}
}

func (m *countingMock) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.received
}

// TestShutdown_CleanDrainsAllEvents sends a large batch through a
// responsive mock collector and verifies every event is transmitted before
// Shutdown returns.
func TestShutdown_CleanDrainsAllEvents(t *testing.T) {
	mock := &countingMock{}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	logger, err := New(
		WithCollectorEndpoint(srv.URL),
		WithServiceName("shutdown-clean-test"),
		WithBatchSize(200),
		WithBatchInterval(100*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const total = 1000
	for i := 0; i < total; i++ {
		logger.Info("clean shutdown event", Int("seq", i))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := logger.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if got := mock.count(); got != total {
		t.Fatalf("mock received %d events, want %d", got, total)
	}
	if dropped := logger.DroppedCount(); dropped != 0 {
		t.Fatalf("DroppedCount = %d, want 0 for a responsive collector", dropped)
	}
}

// TestShutdown_SlowCollectorRespectsDeadline verifies that with a slow but
// working collector, Shutdown completes within its deadline and every
// event is accounted for (transmitted or counted as dropped) — none are
// silently lost.
func TestShutdown_SlowCollectorRespectsDeadline(t *testing.T) {
	mock := &countingMock{delay: 100 * time.Millisecond}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	logger, err := New(
		WithCollectorEndpoint(srv.URL),
		WithServiceName("shutdown-slow-test"),
		WithBatchSize(500),
		WithBatchInterval(100*time.Millisecond),
		WithHTTPTimeout(5*time.Second), // generous: the 100ms delay must not itself trip a timeout
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const total = 5000
	for i := 0; i < total; i++ {
		logger.Info("slow collector event", Int("seq", i))
	}

	const deadline = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	start := time.Now()
	err = logger.Shutdown(ctx)
	elapsed := time.Since(start)

	if elapsed > deadline+500*time.Millisecond {
		t.Fatalf("Shutdown took %v, want within ~%v of the %v deadline", elapsed, 500*time.Millisecond, deadline)
	}

	received := int64(mock.count())
	dropped := logger.DroppedCount()
	if received+dropped != total {
		t.Fatalf("received %d + dropped %d = %d, want %d (events unaccounted for)",
			received, dropped, received+dropped, total)
	}
	t.Logf("shutdown err=%v elapsed=%v received=%d dropped=%d", err, elapsed, received, dropped)
}

// TestShutdown_DeadCollectorReturnsPromptly verifies that when the
// collector is entirely unreachable, Shutdown still returns within its
// deadline (plus a small epsilon) rather than hanging, and that no
// goroutines are left running afterwards.
func TestShutdown_DeadCollectorReturnsPromptly(t *testing.T) {
	// A listener that is opened then immediately closed: the address is
	// syntactically valid but nothing answers on it, so connections fail
	// fast with "connection refused" instead of hanging on a real timeout.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	deadAddr := "http://" + ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("closing listener: %v", err)
	}

	logger, err := New(
		WithCollectorEndpoint(deadAddr),
		WithServiceName("shutdown-dead-test"),
		WithBatchSize(500),
		WithBatchInterval(100*time.Millisecond),
		WithHTTPTimeout(2*time.Second),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const total = 100
	for i := 0; i < total; i++ {
		logger.Info("dead collector event", Int("seq", i))
	}

	const deadline = 3 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	start := time.Now()
	_ = logger.Shutdown(ctx) // error is best-effort here; timing/accounting are what we assert
	elapsed := time.Since(start)

	if elapsed > deadline+500*time.Millisecond {
		t.Fatalf("Shutdown against a dead collector took %v, want within ~%v of the %v deadline",
			elapsed, 500*time.Millisecond, deadline)
	}

	// Every event that could not be transmitted must be counted as dropped
	// — this collector never accepts a single request, so all of them are.
	if dropped := logger.DroppedCount(); dropped != total {
		t.Fatalf("DroppedCount = %d, want %d (collector never accepted anything)", dropped, total)
	}

	// Local, targeted leak check in addition to the package-wide TestMain
	// guard — this is the scenario most likely to wedge a goroutine
	// (retry loop racing shutdown), so it gets its own explicit assertion.
	if err := goleak.Find(
		goleak.IgnoreTopFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).writeLoop"),
	); err != nil {
		t.Fatalf("goroutine leak after dead-collector shutdown: %v", err)
	}
}

// TestShutdown_Idempotent verifies repeated and concurrent Shutdown calls
// are safe and return the same outcome.
func TestShutdown_Idempotent(t *testing.T) {
	mock := &countingMock{}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	logger, err := New(
		WithCollectorEndpoint(srv.URL),
		WithServiceName("shutdown-idempotent-test"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("one event")

	ctx := context.Background()
	var wg sync.WaitGroup
	var errCount atomic.Int32
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := logger.Shutdown(ctx); err != nil {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if n := errCount.Load(); n != 0 {
		t.Fatalf("%d of 10 concurrent Shutdown calls returned an error", n)
	}
	// A third, sequential call must also be a safe no-op.
	if err := logger.Shutdown(ctx); err != nil {
		t.Fatalf("post-concurrent Shutdown: %v", err)
	}
}

// TestShutdown_DefaultDeadlineAppliesToBackgroundContext verifies that
// Shutdown(context.Background()) does not block forever against an
// unreachable collector — WithShutdownTimeout's default bounds it.
func TestShutdown_DefaultDeadlineAppliesToBackgroundContext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	deadAddr := "http://" + ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("closing listener: %v", err)
	}

	logger, err := New(
		WithCollectorEndpoint(deadAddr),
		WithServiceName("shutdown-default-deadline-test"),
		WithShutdownTimeout(2*time.Second), // short, so the test itself stays fast
		WithHTTPTimeout(1*time.Second),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("event")

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- logger.Shutdown(context.Background()) }()

	select {
	case err := <-done:
		elapsed := time.Since(start)
		if elapsed > 3*time.Second {
			t.Fatalf("Shutdown(context.Background()) took %v, want bounded by ~2s ShutdownTimeout", elapsed)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Logf("Shutdown returned %v (deadline-bound path exercised; exact error is best-effort)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown(context.Background()) did not return — default deadline not applied")
	}
}

// TestRegisterSignalHandler_StopTriggersShutdown exercises the
// registration/cancellation code path. Actually delivering a real
// SIGINT/SIGTERM to verify end-to-end signal-triggered shutdown is
// deliberately not done here: self-signalling is not portable across the
// platforms this SDK targets (Windows in particular does not support
// process self-signalling the way POSIX does), so it would make this
// suite flaky rather than more correct. signal.NotifyContext itself is a
// stdlib primitive covered by Go's own tests.
//
// What this test verifies is the documented (and, on first look,
// counter-intuitive) behaviour: the returned stop function is
// signal.NotifyContext's cancel func, so calling it closes the same Done
// channel a real signal would — it is NOT a side-effect-free
// deregistration, and it DOES trigger Shutdown. An earlier version of
// this test asserted the opposite and only passed by scheduling luck (the
// async goroutine hadn't run yet at assertion time); polling here instead
// of a bare immediate check is what makes the assertion meaningful.
func TestRegisterSignalHandler_StopTriggersShutdown(t *testing.T) {
	logger, err := New(
		WithCollectorEndpoint("http://127.0.0.1:1"), // never dialed — no events are logged
		WithServiceName("signal-handler-test"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = logger.Shutdown(context.Background()) }()

	stop := logger.RegisterSignalHandler()
	if stop == nil {
		t.Fatal("RegisterSignalHandler returned a nil stop function")
	}
	stop() // must not panic; closes the handler's context like a real signal would

	deadline := time.Now().Add(2 * time.Second)
	for !logger.closed.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !logger.closed.Load() {
		t.Fatal("calling stop() did not trigger Shutdown within 2s")
	}
}
