package opentrace

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/opentrace/opentrace-go/internal/batcher"
	"github.com/opentrace/opentrace-go/internal/exporter"
	"github.com/opentrace/opentrace-go/internal/retry"
	"github.com/opentrace/opentrace-go/internal/wire"
)

// newSendPolicy composes the transmission policy applied around every batch
// POST: a circuit breaker (host protection during collector outages, ADR-005
// data-loss point 3) wrapping exponential-backoff retries with jitter
// (data-loss point 4).
func newSendPolicy(cfg *Config) func(context.Context, func(context.Context) error) error {
	backoff := retry.BackoffConfig{
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    30 * time.Second,
		MaxJitter:   time.Second,
		MaxAttempts: cfg.MaxRetries,
	}
	if cfg.Debug {
		backoff.OnRetry = func(attempt int, err error, wait time.Duration) {
			fmt.Fprintf(os.Stderr, "[opentrace-sdk] retry %d after %v: %v\n", attempt+1, wait, err)
		}
	}
	breaker := retry.NewCircuitBreaker(5, 60*time.Second)
	return func(ctx context.Context, fn func(context.Context) error) error {
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

	batcherDone chan struct{} // closed to tell the batcher to sweep and exit
	batcherFin  chan struct{} // closed when the batcher goroutine has exited
	cancel      context.CancelFunc

	stopOnce sync.Once
	stopErr  error
}

// newPipeline wires buffer → batcher → flushCh → exporter from cfg.
// onDrop feeds every drop point into the Logger's dropped counter.
func newPipeline(cfg *Config, buf *eventBuffer, res resourceInfo, onDrop func(int64)) *pipeline {
	flushCh := make(chan []*wire.LogEvent, 16)
	return &pipeline{
		flushCh: flushCh,
		bat: batcher.New(batcher.Config{
			MaxSize:     cfg.BatchSize,
			MaxBytes:    cfg.MaxBatchBytes,
			MaxInterval: cfg.BatchInterval,
			In:          buf.ch,
			Out:         flushCh,
			OnDrop:      onDrop,
		}),
		exp: exporter.New(exporter.Config{
			Endpoint:           cfg.CollectorEndpoint,
			Headers:            cfg.Headers,
			SDKVersion:         Version,
			HTTPTimeout:        cfg.HTTPTimeout,
			CompressionEnabled: cfg.CompressionEnabled,
			Debug:              cfg.Debug,
			FlushCh:            flushCh,
			Resource:           res,
			OnDrop:             onDrop,
			Send:               newSendPolicy(cfg),
		}),
		batcherDone: make(chan struct{}),
		batcherFin:  make(chan struct{}),
	}
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
	})
	return p.stopErr
}
