package opentrace

import (
	"testing"
)

// Benchmarks measure the hot path at steady state: a consumer goroutine
// drains the buffer and releases events back to the pool, exactly as the
// background exporter does in production. Run with:
//
//	go test -bench=. -benchmem
//
// Budget (ADR-005): 0 allocs/op and 0 B/op for every scalar-field benchmark.

// benchLogger starts a drain goroutine that recycles events into the pool,
// modelling the exporter. The goroutine exits when the buffer channel closes.
func benchLogger(b *testing.B) *Logger {
	b.Helper()
	l := newTestLogger(LevelInfo, 4096)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range l.buffer.ch {
			releaseEvent(e)
		}
	}()
	b.Cleanup(func() {
		close(l.buffer.ch)
		<-done
	})
	return l
}

func BenchmarkLoggerInfo_NoFields(b *testing.B) {
	l := benchLogger(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("benchmark message")
	}
}

func BenchmarkLoggerInfo_ThreeStringFields(b *testing.B) {
	l := benchLogger(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("benchmark message",
			String("user_id", "u_123"),
			String("request_id", "req-42"),
			String("region", "eu-west-1"),
		)
	}
}

func BenchmarkLoggerInfo_MixedFields(b *testing.B) {
	l := benchLogger(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("benchmark message",
			String("user_id", "u_123"),
			Int("item_count", 3),
			Float64("total", 99.95),
			Bool("cache_hit", true),
			Duration("elapsed", 1500),
		)
	}
}

func BenchmarkLoggerInfo_BelowLevel(b *testing.B) {
	l := benchLogger(b) // MinLevel info; Debug is filtered before the pool
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Debug("filtered", String("k", "v"))
	}
}

func BenchmarkLoggerWith(b *testing.B) {
	l := benchLogger(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = l.With(String("request_id", "r1"))
	}
}

func BenchmarkStringField(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = String("key", "value")
	}
}
