package retry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"testing"
	"time"
)

// httpErr mimics exporter.HTTPError without importing it.
type httpErr struct {
	status     int
	retryAfter time.Duration
}

func (e *httpErr) Error() string                 { return fmt.Sprintf("HTTP %d", e.status) }
func (e *httpErr) HTTPStatus() int               { return e.status }
func (e *httpErr) RetryAfterHint() time.Duration { return e.retryAfter }

func TestBackoffTiming(t *testing.T) {
	cfg := BackoffConfig{
		BaseDelay: 100 * time.Millisecond,
		MaxDelay:  30 * time.Second,
		MaxJitter: time.Nanosecond, // effectively disable jitter for exactness
	}
	wants := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
	}
	for attempt, want := range wants {
		got := ExponentialBackoff(attempt, cfg)
		if got < want || got > want+time.Millisecond {
			t.Errorf("attempt %d: backoff = %v, want ~%v", attempt, got, want)
		}
	}
	// Deep attempts clamp to MaxDelay (also guards shift overflow).
	if got := ExponentialBackoff(40, cfg); got < 30*time.Second || got > 30*time.Second+time.Millisecond {
		t.Errorf("attempt 40: backoff = %v, want ~30s (clamped)", got)
	}
}

func TestJitterRange(t *testing.T) {
	cfg := BackoffConfig{
		BaseDelay: time.Millisecond,
		MaxDelay:  time.Second,
		MaxJitter: 50 * time.Millisecond,
	}
	base := time.Millisecond
	for i := 0; i < 1000; i++ {
		got := ExponentialBackoff(0, cfg)
		jitter := got - base
		if jitter < 0 || jitter >= 50*time.Millisecond {
			t.Fatalf("sample %d: jitter %v outside [0, 50ms)", i, jitter)
		}
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"context cancelled", context.Canceled, false},
		{"deadline exceeded", context.DeadlineExceeded, false},
		{"url.Error network failure", &url.Error{Op: "Post", URL: "http://x", Err: errors.New("refused")}, true},
		{"EOF", io.EOF, true},
		{"unexpected EOF", io.ErrUnexpectedEOF, true},
		{"HTTP 429", &httpErr{status: 429}, true},
		{"HTTP 500", &httpErr{status: 500}, true},
		{"HTTP 502", &httpErr{status: 502}, true},
		{"HTTP 503", &httpErr{status: 503}, true},
		{"HTTP 504", &httpErr{status: 504}, true},
		{"HTTP 400", &httpErr{status: 400}, false},
		{"HTTP 401", &httpErr{status: 401}, false},
		{"HTTP 403", &httpErr{status: 403}, false},
		{"HTTP 404", &httpErr{status: 404}, false},
		{"HTTP 413", &httpErr{status: 413}, false},
		{"wrapped 503", fmt.Errorf("send: %w", &httpErr{status: 503}), true},
		{"plain error", errors.New("mystery"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func fastCfg() BackoffConfig {
	return BackoffConfig{
		BaseDelay:   time.Millisecond,
		MaxDelay:    50 * time.Millisecond,
		MaxJitter:   time.Millisecond,
		MaxAttempts: 5,
	}
}

func TestRetryOnTransientError(t *testing.T) {
	attempts := 0
	err := WithRetry(context.Background(), fastCfg(), func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return &httpErr{status: 503}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithRetry = %v, want nil", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3 (503, 503, 202)", attempts)
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	attempts := 0
	err := WithRetry(context.Background(), fastCfg(), func(ctx context.Context) error {
		attempts++
		return &httpErr{status: 400}
	})
	var sc *httpErr
	if !errors.As(err, &sc) || sc.status != 400 {
		t.Fatalf("WithRetry = %v, want the 400 error", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want exactly 1", attempts)
	}
}

func TestRetryExhaustionWrapsSentinel(t *testing.T) {
	attempts := 0
	err := WithRetry(context.Background(), fastCfg(), func(ctx context.Context) error {
		attempts++
		return &httpErr{status: 503}
	})
	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Fatalf("WithRetry = %v, want ErrMaxRetriesExceeded", err)
	}
	var sc *httpErr
	if !errors.As(err, &sc) {
		t.Fatal("last error not wrapped alongside the sentinel")
	}
	if attempts != 5 {
		t.Fatalf("attempts = %d, want MaxAttempts (5)", attempts)
	}
}

func TestRetryHonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	cfg := fastCfg()
	cfg.BaseDelay = 10 * time.Second // force a long wait after attempt 1
	cfg.MaxJitter = time.Millisecond
	start := time.Now()
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := WithRetry(ctx, cfg, func(ctx context.Context) error {
		attempts++
		return &httpErr{status: 503}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WithRetry = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("cancellation took %v — retry ignored ctx during backoff wait", elapsed)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (cancelled during first wait)", attempts)
	}
}

func TestRetryAfterHintOverridesBackoff(t *testing.T) {
	attempts := 0
	waits := []time.Duration{}
	cfg := fastCfg()
	cfg.OnRetry = func(_ int, _ error, wait time.Duration) { waits = append(waits, wait) }
	err := WithRetry(context.Background(), cfg, func(ctx context.Context) error {
		attempts++
		if attempts == 1 {
			return &httpErr{status: 429, retryAfter: 30 * time.Millisecond}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithRetry = %v", err)
	}
	if len(waits) != 1 || waits[0] != 30*time.Millisecond {
		t.Fatalf("waits = %v, want [30ms] from Retry-After", waits)
	}
}

func TestRetryAfterHintCappedAtMaxDelay(t *testing.T) {
	cfg := fastCfg() // MaxDelay 50ms
	var gotWait time.Duration
	cfg.OnRetry = func(_ int, _ error, wait time.Duration) { gotWait = wait }
	attempts := 0
	_ = WithRetry(context.Background(), cfg, func(ctx context.Context) error {
		attempts++
		if attempts == 1 {
			return &httpErr{status: 429, retryAfter: time.Hour}
		}
		return nil
	})
	if gotWait != 50*time.Millisecond {
		t.Fatalf("wait = %v, want MaxDelay cap (50ms)", gotWait)
	}
}

// fakeClock drives the breaker's time without sleeping.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestBreaker(maxFailures int, resetTimeout time.Duration) (*CircuitBreaker, *fakeClock) {
	cb := NewCircuitBreaker(maxFailures, resetTimeout)
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	cb.now = clk.now
	return cb, clk
}

func failing(ctx context.Context) error    { return &httpErr{status: 503} }
func succeeding(ctx context.Context) error { return nil }

func TestCircuitBreakerOpensAfterMaxFailures(t *testing.T) {
	cb, _ := newTestBreaker(5, time.Minute)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := cb.Do(ctx, failing); errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("breaker opened early at failure %d", i+1)
		}
	}
	// Sixth call: breaker is open, operation must not run.
	ran := false
	err := cb.Do(ctx, func(ctx context.Context) error { ran = true; return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("Do = %v, want ErrCircuitOpen", err)
	}
	if ran {
		t.Fatal("operation ran while the circuit was open")
	}
}

func TestCircuitBreakerHalfOpenRecovery(t *testing.T) {
	cb, clk := newTestBreaker(2, time.Minute)
	ctx := context.Background()
	_ = cb.Do(ctx, failing)
	_ = cb.Do(ctx, failing) // opens
	if err := cb.Do(ctx, succeeding); !errors.Is(err, ErrCircuitOpen) {
		t.Fatal("breaker did not open after maxFailures")
	}

	clk.advance(61 * time.Second) // past resetTimeout → half-open probe allowed
	if err := cb.Do(ctx, succeeding); err != nil {
		t.Fatalf("half-open probe = %v, want success", err)
	}
	// Closed again: calls flow normally.
	if err := cb.Do(ctx, succeeding); err != nil {
		t.Fatalf("post-recovery call = %v", err)
	}
}

func TestCircuitBreakerHalfOpenFailureReopens(t *testing.T) {
	cb, clk := newTestBreaker(2, time.Minute)
	ctx := context.Background()
	_ = cb.Do(ctx, failing)
	_ = cb.Do(ctx, failing) // opens

	clk.advance(61 * time.Second)
	_ = cb.Do(ctx, failing) // half-open probe fails → re-opens immediately
	if err := cb.Do(ctx, succeeding); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("Do = %v, want ErrCircuitOpen after failed probe", err)
	}
	// And it can still recover after another full window.
	clk.advance(61 * time.Second)
	if err := cb.Do(ctx, succeeding); err != nil {
		t.Fatalf("second probe = %v, want success", err)
	}
}

func TestCircuitBreakerSuccessResetsFailureCount(t *testing.T) {
	cb, _ := newTestBreaker(3, time.Minute)
	ctx := context.Background()
	_ = cb.Do(ctx, failing)
	_ = cb.Do(ctx, failing)
	_ = cb.Do(ctx, succeeding) // resets the consecutive-failure count
	_ = cb.Do(ctx, failing)
	_ = cb.Do(ctx, failing)
	// Still closed: only 2 consecutive failures since the success.
	if err := cb.Do(ctx, succeeding); errors.Is(err, ErrCircuitOpen) {
		t.Fatal("breaker opened despite non-consecutive failures")
	}
}
