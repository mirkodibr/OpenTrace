// Package retry provides exponential backoff with jitter and a three-state
// circuit breaker for the SDK's export path. It is fully generic — it
// classifies errors through small interfaces and never imports wire or
// exporter types.
//
// # Data loss policy
//
// The SDK deliberately DROPS telemetry rather than block the host
// application. There are exactly four drop points:
//
//  1. Hot-path buffer full (logger): the incoming event is dropped.
//  2. Flush channel full (batcher): the entire batch is dropped.
//  3. Circuit breaker OPEN (this package): the batch is dropped without a
//     network attempt — this is the host-protection boundary during a
//     collector outage.
//  4. Retries exhausted (this package): the batch is dropped after the
//     final attempt fails.
//
// Every drop increments the counter behind Logger.DroppedCount(); operators
// should alert on its growth rate. The trade-off is deliberate: a 1-second
// stall inside logger.Info() in a request handler adds a full second to
// that request's p99 latency. Telemetry loss is observable and recoverable;
// host latency injected by its own logging library is neither.
//
// # Why jitter is mandatory
//
// Without jitter, deterministic backoff synchronises failures: if 1,000 SDK
// instances lose the collector simultaneously, they all retry at base×2^n
// at the same instants, hammering the recovering collector with correlated
// bursts exactly when it is weakest (thundering herd). A uniform random
// jitter term decorrelates the retry schedule across instances, spreading
// the recovery load.
package retry

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net/url"
	"sync"
	"time"
)

// ErrMaxRetriesExceeded wraps the last error after all attempts fail.
var ErrMaxRetriesExceeded = errors.New("opentrace: max retries exceeded")

// ErrCircuitOpen is returned by CircuitBreaker.Do without invoking the
// operation while the breaker is open.
var ErrCircuitOpen = errors.New("opentrace: circuit breaker is open")

// BackoffConfig parameterises WithRetry.
//
// Wait times with the default config (BaseDelay 100ms, MaxDelay 30s,
// MaxJitter 1s):
//
//	| Attempt | Base wait | With max jitter | Cumulative max |
//	|---------|-----------|-----------------|----------------|
//	| 0       | 100 ms    | 1.1 s           | 1.1 s          |
//	| 1       | 200 ms    | 1.2 s           | 2.3 s          |
//	| 2       | 400 ms    | 1.4 s           | 3.7 s          |
//	| 3       | 800 ms    | 1.8 s           | 5.5 s          |
//	| 4       | 1.6 s     | 2.6 s           | 8.1 s          |
//	| 5       | 3.2 s     | 4.2 s           | 12.3 s         |
type BackoffConfig struct {
	BaseDelay   time.Duration // default 100ms
	MaxDelay    time.Duration // default 30s
	MaxJitter   time.Duration // default 1s
	MaxAttempts int           // default 5 (total attempts, not retries)
	// OnRetry, if set, is called before each wait with the attempt number
	// (0-based), the error that triggered the retry, and the wait duration.
	OnRetry func(attempt int, err error, wait time.Duration)
}

func (c BackoffConfig) withDefaults() BackoffConfig {
	if c.BaseDelay <= 0 {
		c.BaseDelay = 100 * time.Millisecond
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = 30 * time.Second
	}
	if c.MaxJitter <= 0 {
		c.MaxJitter = time.Second
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	return c
}

// ExponentialBackoff computes the wait before retry number attempt (0-based):
//
//	wait = min(MaxDelay, BaseDelay × 2^attempt) + uniform[0, MaxJitter)
func ExponentialBackoff(attempt int, cfg BackoffConfig) time.Duration {
	cfg = cfg.withDefaults()
	base := cfg.BaseDelay << uint(attempt) // BaseDelay × 2^attempt
	if base <= 0 || base > cfg.MaxDelay {  // <=0 catches shift overflow
		base = cfg.MaxDelay
	}
	return base + rand.N(cfg.MaxJitter)
}

// statusCarrier is implemented by transport errors that carry an HTTP
// status (e.g. exporter.HTTPError). Declared here so this package needs no
// import of the exporter.
type statusCarrier interface {
	HTTPStatus() int
	RetryAfterHint() time.Duration
}

// IsRetryable reports whether err is worth retrying.
//
// Retryable: network-level failures (*url.Error), connection resets
// (io.EOF, io.ErrUnexpectedEOF), HTTP 429 and 500/502/503/504.
// Not retryable: context cancellation/expiry, HTTP 4xx client errors
// (400/401/403/404/413…) — repeating a rejected request cannot succeed.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var sc statusCarrier
	if errors.As(err, &sc) {
		switch sc.HTTPStatus() {
		case 429, 500, 502, 503, 504:
			return true
		default:
			return false
		}
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return true
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// retryAfterHint extracts a server-provided wait (Retry-After) if present,
// capped at MaxDelay.
func retryAfterHint(err error, cfg BackoffConfig) (time.Duration, bool) {
	var sc statusCarrier
	if errors.As(err, &sc) {
		if ra := sc.RetryAfterHint(); ra > 0 {
			if ra > cfg.MaxDelay {
				ra = cfg.MaxDelay
			}
			return ra, true
		}
	}
	return 0, false
}

// WithRetry executes fn with exponential backoff until it succeeds, a
// non-retryable error occurs, the attempt budget is spent, or ctx ends.
func WithRetry(ctx context.Context, cfg BackoffConfig, fn func(context.Context) error) error {
	cfg = cfg.withDefaults()
	var lastErr error
	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}
		if !IsRetryable(lastErr) {
			return lastErr
		}
		if attempt == cfg.MaxAttempts-1 {
			break // budget spent; don't wait after the final attempt
		}
		wait := ExponentialBackoff(attempt, cfg)
		if hint, ok := retryAfterHint(lastErr, cfg); ok {
			wait = hint // the server told us when to come back
		}
		if cfg.OnRetry != nil {
			cfg.OnRetry(attempt, lastErr, wait)
		}
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
	return errors.Join(ErrMaxRetriesExceeded, lastErr)
}

// breakerState is the classic three-state machine.
type breakerState uint8

const (
	stateClosed breakerState = iota
	stateOpen
	stateHalfOpen
)

// CircuitBreaker trips OPEN after MaxFailures consecutive failures; while
// open it fails fast without invoking the operation. After ResetTimeout it
// admits a single probe (HALF-OPEN): success closes the circuit, failure
// re-opens it. The mutex is uncontended in practice — Do is called almost
// exclusively from the exporter goroutine — but guards against the
// concurrent Export paths (drain loop vs. Fatal flush).
type CircuitBreaker struct {
	maxFailures  int
	resetTimeout time.Duration
	now          func() time.Time // injectable for tests

	mu       sync.Mutex
	state    breakerState
	failures int
	openedAt time.Time
}

// NewCircuitBreaker constructs a breaker. maxFailures <= 0 defaults to 5;
// resetTimeout <= 0 defaults to 60s.
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	if maxFailures <= 0 {
		maxFailures = 5
	}
	if resetTimeout <= 0 {
		resetTimeout = 60 * time.Second
	}
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		now:          time.Now,
	}
}

// Do runs fn through the breaker.
func (cb *CircuitBreaker) Do(ctx context.Context, fn func(context.Context) error) error {
	if !cb.admit() {
		return ErrCircuitOpen
	}
	err := fn(ctx)
	cb.record(err == nil)
	return err
}

// admit decides whether a call may proceed, transitioning OPEN → HALF-OPEN
// when the reset window has elapsed.
func (cb *CircuitBreaker) admit() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case stateClosed, stateHalfOpen:
		return true
	case stateOpen:
		if cb.now().Sub(cb.openedAt) >= cb.resetTimeout {
			cb.state = stateHalfOpen
			return true
		}
		return false
	default:
		return true
	}
}

// record applies the state transitions for a completed call.
func (cb *CircuitBreaker) record(success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if success {
		cb.state = stateClosed
		cb.failures = 0
		return
	}
	switch cb.state {
	case stateHalfOpen:
		cb.state = stateOpen
		cb.openedAt = cb.now()
	case stateClosed:
		cb.failures++
		if cb.failures >= cb.maxFailures {
			cb.state = stateOpen
			cb.openedAt = cb.now()
		}
	}
}
