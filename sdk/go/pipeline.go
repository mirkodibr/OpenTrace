package opentrace

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opentrace/opentrace-go/internal/batcher"
	"github.com/opentrace/opentrace-go/internal/exporter"
	"github.com/opentrace/opentrace-go/internal/retry"
	"github.com/opentrace/opentrace-go/internal/wire"
)

// shutdownMaxRetries is the reduced retry budget applied to every export
// attempt once shutdown has begun (Day 33 timeout analysis): a full retry
// cycle at the default policy (HTTPTimeout x MaxRetries, up to 8s x 5 = 40s)
// vastly exceeds any reasonable shutdown deadline, so shutdown trades
// persistence for timeliness — 2 attempts still absorbs one transient
// blip without risking the shutdown budget.
const shutdownMaxRetries = 2

// newSendPolicy composes the transmission policy applied around every batch
// POST: a circuit breaker (host protection during collector outages, ADR-005
// data-loss point 3) wrapping exponential-backoff retries with jitter
// (data-loss point 4). shuttingDown is checked on every call so exports that
// start after shutdown begins use the reduced retry budget; an export
// already retrying when shutdown begins keeps its original budget for that
// call — this is a documented, accepted edge case, not a bug.
func newSendPolicy(cfg *Config, shuttingDown *atomic.Bool) func(context.Context, func(context.Context) error) error {
	normal := retry.BackoffConfig{
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    30 * time.Second,
		MaxJitter:   time.Second,
		MaxAttempts: cfg.MaxRetries,
	}
	draining := normal
	draining.MaxAttempts = shutdownMaxRetries
	if cfg.Debug {
		onRetry := func(attempt int, err error, wait time.Duration) {
			fmt.Fprintf(os.Stderr, "[opentrace-sdk] retry %d after %v: %v\n", attempt+1, wait, err)
		}
		normal.OnRetry = onRetry
		draining.OnRetry = onRetry
	}
	breaker := retry.NewCircuitBreaker(5, 60*time.Second)
	return func(ctx context.Context, fn func(context.Context) error) error {
		backoff := normal
		if shuttingDown.Load() {
			backoff = draining
		}
		return breaker.Do(ctx, func(ctx context.Context) error {
			return retry.WithRetry(ctx, backoff, fn)
		})
	}
}

// pipeline owns the SDK's two background goroutines (batcher and exporter)
// and their orderly shutdown (ADR-005 D1). Nothing else in the SDK may spawn
// goroutines.
type pipeline struct {
	bat     *batcher.Batcher
	exp     *exporter.HTTPExporter
	flushCh chan []*wire.LogEvent

	shuttingDown atomic.Bool // set first thing in stop(); read by the send policy
	debug        bool
	dropped      *atomic.Int64 // shared with Logger; read for the shutdown summary

	batcherDone chan struct{} // closed to tell the batcher to sweep and exit
	batcherFin  chan struct{} // closed when the batcher goroutine has exited
	cancel      context.CancelFunc

	stopOnce sync.Once
	stopErr  error
}

// newPipeline wires buffer → batcher → flushCh → exporter from cfg. dropped
// is the Logger's shared counter: every drop point (buffer full, flushCh
// full, circuit open, retries exhausted) feeds into it, and stop() reads it
// back for the shutdown summary.
func newPipeline(cfg *Config, buf *eventBuffer, res resourceInfo, dropped *atomic.Int64) *pipeline {
	flushCh := make(chan []*wire.LogEvent, 16)
	p := &pipeline{
		flushCh:     flushCh,
		debug:       cfg.Debug,
		dropped:     dropped,
		batcherDone: make(chan struct{}),
		batcherFin:  make(chan struct{}),
	}
	onDrop := func(n int64) { dropped.Add(n) }
	p.bat = batcher.New(batcher.Config{
		MaxSize:     cfg.BatchSize,
		MaxBytes:    cfg.MaxBatchBytes,
		MaxInterval: cfg.BatchInterval,
		In:          buf.ch,
		Out:         flushCh,
		OnDrop:      onDrop,
	})
	p.exp = exporter.New(exporter.Config{
		Endpoint:           cfg.CollectorEndpoint,
		Headers:            cfg.Headers,
		SDKVersion:         Version,
		HTTPTimeout:        cfg.HTTPTimeout,
		CompressionEnabled: cfg.CompressionEnabled,
		Debug:              cfg.Debug,
		FlushCh:            flushCh,
		Resource:           res,
		OnDrop:             onDrop,
		Send:               newSendPolicy(cfg, &p.shuttingDown),
	})
	return p
}

// start launches the batcher and exporter goroutines. Called by New only
// after config validation has passed.
func (p *pipeline) start() {
	runCtx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go func() {
		defer close(p.batcherFin)
		p.bat.Run(p.batcherDone)
	}()
	p.exp.Start(runCtx)
}

// stop executes the ordered drain (ADR-005 shutdown ordering):
//
//	producers gated (caller) → batcher sweeps buffer and flushes the final
//	batch → batcher closes flushCh (sole sender) → exporter drains flushCh
//	to completion → done.
//
// The multi-producer buffer channel is never closed — a concurrent hot-path
// send to a closed channel would panic. If ctx expires before the drain
// completes, in-flight HTTP work is cancelled and ctx.Err() is returned;
// events still queued at that point are abandoned (counted best-effort by
// the exporter's drop path).
func (p *pipeline) stop(ctx context.Context) error {
	p.stopOnce.Do(func() {
		// Set BEFORE signalling the batcher: any export that starts from
		// this point on (including the batcher's own final-sweep flush)
		// uses the reduced shutdown retry budget.
		p.shuttingDown.Store(true)
		close(p.batcherDone)

		drained := make(chan struct{})
		go func() {
			<-p.batcherFin
			p.exp.Wait()
			close(drained)
		}()

		select {
		case <-drained:
		case <-ctx.Done():
			p.cancel() // abort in-flight HTTP requests
			select {
			case <-drained:
			case <-time.After(2 * time.Second):
				// The drain loop is wedged beyond the grace window;
				// abandon it rather than hang the host's shutdown.
			}
			p.stopErr = ctx.Err()
		}

		p.exp.Shutdown()
		p.cancel()

		if p.debug {
			_, sent, _ := p.exp.Stats()
			fmt.Fprintf(os.Stderr,
				"[opentrace-sdk] shutdown complete: %d events transmitted, %d events dropped\n",
				sent, p.dropped.Load())
		}
	})
	return p.stopErr
}
