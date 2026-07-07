package batcher

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opentrace/opentrace-go/internal/wire"
)

func newEvent(msg string) *wire.LogEvent {
	e := wire.AcquireEvent()
	e.Level = wire.LevelInfo
	e.Message = msg
	e.Timestamp = time.Now()
	return e
}

// harness wires a batcher with buffered in/out channels and runs it until
// the returned stop function is called.
type harness struct {
	in      chan *wire.LogEvent
	out     chan []*wire.LogEvent
	dropped atomic.Int64
	stop    func()
}

func newHarness(t *testing.T, maxSize, maxBytes int, maxInterval time.Duration, outCap int) *harness {
	t.Helper()
	h := &harness{
		in:  make(chan *wire.LogEvent, 1024),
		out: make(chan []*wire.LogEvent, outCap),
	}
	b := New(Config{
		MaxSize:     maxSize,
		MaxBytes:    maxBytes,
		MaxInterval: maxInterval,
		In:          h.in,
		Out:         h.out,
		OnDrop:      func(n int64) { h.dropped.Add(n) },
	})
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		b.Run(done)
	}()
	var once sync.Once
	h.stop = func() {
		once.Do(func() {
			close(done)
			<-finished
		})
	}
	t.Cleanup(h.stop)
	return h
}

// waitBatch receives one batch from out with a timeout.
func (h *harness) waitBatch(t *testing.T, timeout time.Duration) []*wire.LogEvent {
	t.Helper()
	select {
	case batch := <-h.out:
		return batch
	case <-time.After(timeout):
		t.Fatal("timed out waiting for a batch")
		return nil
	}
}

func TestBatchFlushOnSize(t *testing.T) {
	h := newHarness(t, 5, 1<<20, time.Hour, 4)
	for i := 0; i < 5; i++ {
		h.in <- newEvent("event")
	}
	batch := h.waitBatch(t, 2*time.Second)
	if len(batch) != 5 {
		t.Fatalf("batch size = %d, want 5", len(batch))
	}
	for _, e := range batch {
		wire.ReleaseEvent(e)
	}
}

func TestBatchFlushOnBytes(t *testing.T) {
	// Each event estimates at ~133 bytes; a 250-byte cap flushes on the
	// second event even though the size cap (1000) is far away.
	h := newHarness(t, 1000, 250, time.Hour, 4)
	h.in <- newEvent("first")
	h.in <- newEvent("second")
	batch := h.waitBatch(t, 2*time.Second)
	if len(batch) != 2 {
		t.Fatalf("batch size = %d, want 2", len(batch))
	}
	for _, e := range batch {
		wire.ReleaseEvent(e)
	}
}

func TestBatchFlushOnTimer(t *testing.T) {
	h := newHarness(t, 1000, 1<<20, 50*time.Millisecond, 4)
	h.in <- newEvent("lonely")
	batch := h.waitBatch(t, 2*time.Second)
	if len(batch) != 1 {
		t.Fatalf("batch size = %d, want 1", len(batch))
	}
	wire.ReleaseEvent(batch[0])
}

func TestConcurrentAddNoLossNoDuplication(t *testing.T) {
	const producers = 100
	const perProducer = 100
	h := newHarness(t, 64, 1<<20, 20*time.Millisecond, 256)

	var wg sync.WaitGroup
	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				h.in <- newEvent("concurrent")
			}
		}()
	}
	wg.Wait()
	h.stop() // final sweep flushes the remainder and closes out

	received := 0
	for batch := range h.out {
		received += len(batch)
		for _, e := range batch {
			wire.ReleaseEvent(e)
		}
	}
	total := received + int(h.dropped.Load())
	if total != producers*perProducer {
		t.Fatalf("received %d + dropped %d = %d, want %d",
			received, h.dropped.Load(), total, producers*perProducer)
	}
	if h.dropped.Load() > 0 {
		t.Logf("note: %d events dropped due to full flush channel (accounted)", h.dropped.Load())
	}
}

func TestFlushChannelFullDropsAndCounts(t *testing.T) {
	// out has capacity 1 and no consumer: the first flush fills it, the
	// second must drop and invoke OnDrop.
	h := newHarness(t, 2, 1<<20, time.Hour, 1)
	for i := 0; i < 4; i++ {
		h.in <- newEvent("overflow")
	}
	// Wait until the drop counter reflects the second batch.
	deadline := time.Now().Add(2 * time.Second)
	for h.dropped.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := h.dropped.Load(); got != 2 {
		t.Fatalf("dropped = %d, want 2", got)
	}
	batch := h.waitBatch(t, time.Second)
	if len(batch) != 2 {
		t.Fatalf("delivered batch size = %d, want 2", len(batch))
	}
	for _, e := range batch {
		wire.ReleaseEvent(e)
	}
}

func TestShutdownSweepFlushesRemainder(t *testing.T) {
	h := newHarness(t, 1000, 1<<20, time.Hour, 4)
	for i := 0; i < 7; i++ {
		h.in <- newEvent("pending")
	}
	// Give the run loop a moment to consume from in.
	time.Sleep(20 * time.Millisecond)
	h.stop()

	received := 0
	for batch := range h.out {
		received += len(batch)
		for _, e := range batch {
			wire.ReleaseEvent(e)
		}
	}
	if received != 7 {
		t.Fatalf("received %d events after shutdown sweep, want 7", received)
	}
}
