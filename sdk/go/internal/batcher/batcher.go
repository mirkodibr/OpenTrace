// Package batcher implements Layer 2 of the SDK pipeline (ADR-005): it
// drains the hot-path event buffer, accumulates events into batches, and
// hands full batches to the exporter through a flush channel.
//
// # Batch sizing (mathematical basis)
//
// Given a target ingest rate R (events/second) and a maximum acceptable
// delivery latency L (seconds), the natural batch size is S = R × L — the
// number of events that accumulate within one latency window. The flush
// interval equals L; the size trigger exists so bursts above R flush early
// instead of growing the batch without bound.
//
//	| Profile               | Rate R     | Max latency L | Batch size S | Interval |
//	|-----------------------|------------|---------------|--------------|----------|
//	| Low-traffic service   | 100 eps    | 5 s           | 500          | 5 s      |
//	| Standard web service  | 1,000 eps  | 2 s           | 2,000→500*   | 2 s      |
//	| High-frequency service| 10,000 eps | 500 ms        | 5,000→500*   | 500 ms   |
//
// (*) capped at the collector's server-side batch limit (1,000) with margin;
// the SDK default of 500 keeps every batch acceptable to the collector while
// the interval trigger bounds latency for low-rate services.
//
// Memory footprint at steady state ≈ 2 × maxSize × avg_event_size (the
// current batch plus the batch in flight after the swap). With 500 events of
// ~512 B that is ~0.5 MB — bounded by construction. Events accumulated in a
// batch that has not flushed when the process crashes are lost; that window
// is at most maxInterval and is an explicit design decision (drop, never
// block or persist on the host's critical path).
package batcher

import (
	"time"

	"github.com/opentrace/opentrace-go/internal/wire"
)

// Config parameterises a Batcher.
type Config struct {
	MaxSize     int           // flush when the batch reaches this many events
	MaxBytes    int           // flush when the estimated batch size reaches this many bytes
	MaxInterval time.Duration // flush when this much time has passed since the last flush
	In          <-chan *wire.LogEvent
	Out         chan<- []*wire.LogEvent
	// OnDrop is invoked with the number of events dropped when Out is full.
	// Must be safe for calls from the batcher goroutine.
	OnDrop func(int64)
}

// Batcher accumulates events and flushes on size, bytes, or interval —
// whichever triggers first. All state is owned by the single goroutine
// running Run; there is no shared mutable state and no locking.
type Batcher struct {
	cfg   Config
	batch []*wire.LogEvent
	bytes int
}

// New constructs a Batcher. Call Run in a dedicated goroutine.
func New(cfg Config) *Batcher {
	if cfg.OnDrop == nil {
		cfg.OnDrop = func(int64) {}
	}
	return &Batcher{
		cfg:   cfg,
		batch: make([]*wire.LogEvent, 0, cfg.MaxSize),
	}
}

// Run drains the input channel until done is closed, then performs a final
// non-blocking sweep of the input, flushes the remaining batch, and closes
// the output channel. Run is the sole sender on cfg.Out, which is what makes
// closing it safe (ADR-005 shutdown ordering).
func (b *Batcher) Run(done <-chan struct{}) {
	ticker := time.NewTicker(b.cfg.MaxInterval)
	defer ticker.Stop()

	for {
		select {
		case e := <-b.cfg.In:
			if b.add(e) {
				b.flush()
				ticker.Reset(b.cfg.MaxInterval)
			}
		case <-ticker.C:
			if len(b.batch) > 0 {
				b.flush()
			}
		case <-done:
			b.sweep()
			b.flush()
			close(b.cfg.Out)
			return
		}
	}
}

// add appends e to the current batch and reports whether a flush trigger
// (size or bytes) fired.
func (b *Batcher) add(e *wire.LogEvent) bool {
	b.batch = append(b.batch, e)
	b.bytes += estimateSize(e)
	return len(b.batch) >= b.cfg.MaxSize || b.bytes >= b.cfg.MaxBytes
}

// flush hands the current batch to the output channel without blocking.
// If the exporter has fallen behind and the channel is full, the batch is
// dropped: every event is released back to the pool and OnDrop is called.
// Blocking here would eventually back up into the hot-path buffer.
func (b *Batcher) flush() {
	if len(b.batch) == 0 {
		return
	}
	out := b.batch
	// Swap in a fresh slice; the old backing array now belongs to the
	// exporter until it releases the events.
	b.batch = make([]*wire.LogEvent, 0, b.cfg.MaxSize)
	b.bytes = 0

	select {
	case b.cfg.Out <- out:
	default:
		for _, e := range out {
			wire.ReleaseEvent(e)
		}
		b.cfg.OnDrop(int64(len(out)))
	}
}

// sweep performs the shutdown drain: it empties the input channel without
// blocking, flushing intermediate batches whenever a size trigger fires.
// The producer side is already gated by the logger's closed flag, so only
// events from concurrent in-flight log() calls can still arrive; the
// non-blocking loop terminates as soon as the channel is momentarily empty.
func (b *Batcher) sweep() {
	for {
		select {
		case e := <-b.cfg.In:
			if b.add(e) {
				b.flush()
			}
		default:
			return
		}
	}
}

// estimateSize approximates the serialised size of an event in bytes. The
// exact size is unknowable before serialisation (which happens in the
// exporter, after batching), so this uses the dominant terms: message and
// field key/value lengths plus a fixed per-event envelope overhead. The
// estimate errs slightly low for numeric fields and high for envelopes;
// MaxBytes is a soft bound intended to keep payloads well under the
// collector's hard 5 MB limit, not an exact accounting.
func estimateSize(e *wire.LogEvent) int {
	const envelopeOverhead = 128 // timestamp, level, service metadata, JSON syntax
	n := envelopeOverhead + len(e.Message)
	for i := range e.Fields {
		f := &e.Fields[i]
		n += len(f.Key) + 8 // key + typical scalar value width
		n += len(f.StringVal)
	}
	return n
}
