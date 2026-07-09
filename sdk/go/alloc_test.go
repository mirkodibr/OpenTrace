//go:build !race

package opentrace

// Allocation-budget enforcement (ADR-005). These tests FAIL the build when a
// zero-alloc guarantee regresses — they are the contract, the benchmarks in
// bench measurement files are the evidence.
//
// They live behind !race because allocation accounting is unreliable under
// the race detector; CI runs a dedicated non-race pass so they always execute.

import (
	"testing"
)

// allocLogger returns a logger with room to enqueue. recycle models the
// steady state: the exporter goroutine continuously releases events back to
// the pool after serialising them, so the pool always has a warm event. The
// tests replicate that by draining and releasing after every call.
func allocLogger() *Logger {
	return newTestLogger(LevelInfo, 8)
}

func recycle(l *Logger) {
	select {
	case e := <-l.buffer.ch:
		releaseEvent(e)
	default:
	}
}

func TestAllocBudget_InfoNoFields(t *testing.T) {
	l := allocLogger()
	if allocs := testing.AllocsPerRun(1000, func() {
		l.Info("steady state message")
		recycle(l)
	}); allocs != 0 {
		t.Errorf("Info(msg) allocates %v allocs/op, budget is 0", allocs)
	}
}

func TestAllocBudget_InfoScalarFields(t *testing.T) {
	l := allocLogger()
	if allocs := testing.AllocsPerRun(1000, func() {
		l.Info("checkout completed",
			String("user_id", "u_123"),
			Int("item_count", 3),
			Float64("total", 99.95),
			Bool("cache_hit", true),
			Duration("db_time", 42),
		)
		recycle(l)
	}); allocs != 0 {
		t.Errorf("Info(msg, scalar fields...) allocates %v allocs/op, budget is 0", allocs)
	}
}

func TestAllocBudget_BelowMinLevel(t *testing.T) {
	l := allocLogger() // MinLevel = info
	if allocs := testing.AllocsPerRun(1000, func() {
		l.Debug("filtered out", String("k", "v"))
	}); allocs != 0 {
		t.Errorf("filtered Debug allocates %v allocs/op, budget is 0", allocs)
	}
}

func TestAllocBudget_FieldConstructors(t *testing.T) {
	if allocs := testing.AllocsPerRun(1000, func() {
		_ = String("key", "value")
		_ = Int("key", 1)
		_ = Int64("key", 1)
		_ = Float64("key", 1.0)
		_ = Bool("key", true)
		_ = Duration("key", 1)
	}); allocs != 0 {
		t.Errorf("scalar field constructors allocate %v allocs/op, budget is 0", allocs)
	}
}

func TestAllocBudget_With(t *testing.T) {
	l := allocLogger()
	// Budget: 1 alloc for the child Logger struct + 1 for the merged fields
	// slice (ADR-005).
	if allocs := testing.AllocsPerRun(1000, func() {
		_ = l.With(String("request_id", "r1"))
	}); allocs > 2 {
		t.Errorf("With(field) allocates %v allocs/op, budget is 2", allocs)
	}
}
