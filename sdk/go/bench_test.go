package opentrace

import (
	"sync"
	"testing"
	"time"
)

// Day 34 load-test benchmark suite: throughput at increasing concurrency
// (1 -> 10 -> 100 goroutines), and the allocation profile of richer field
// sets. Reuses benchLogger (logger_bench_test.go), which drains and
// recycles events through the pool exactly as the real exporter does, so
// these numbers reflect steady-state behaviour, not an artificially
// unbounded buffer.
//
// Run: go test -bench=BenchmarkSDK -benchmem -count=3 ./sdk/go/...
// Results feed docs/internal/runbooks/sdk-load-test-report.md.

// runParallel spawns exactly `goroutines` goroutines (independent of
// GOMAXPROCS — deliberately not using b.SetParallelism, whose goroutine
// count scales with GOMAXPROCS rather than matching the literal "N
// goroutines" scenarios the Day 34 spec calls for) and divides b.N calls
// evenly across them.
func runParallel(b *testing.B, goroutines int, call func()) {
	b.Helper()
	per := b.N / goroutines
	if per == 0 {
		per = 1
	}
	var wg sync.WaitGroup
	b.ResetTimer()
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				call()
			}
		}()
	}
	wg.Wait()
	b.StopTimer()
	reportThroughput(b, goroutines*per)
}

// reportThroughput adds a calls/sec custom metric alongside the standard
// ns/op, so results read directly against the Day 34 throughput targets
// without manual inversion.
func reportThroughput(b *testing.B, totalCalls int) {
	b.Helper()
	elapsed := b.Elapsed()
	if elapsed <= 0 {
		return
	}
	b.ReportMetric(float64(totalCalls)/elapsed.Seconds(), "calls/sec")
}

// BenchmarkSDK_SingleGoroutine: target >= 1,000,000 log calls/second,
// 0 allocs/op (the ADR-005 budget).
func BenchmarkSDK_SingleGoroutine(b *testing.B) {
	l := benchLogger(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("load test message", String("k", "v"))
	}
	b.StopTimer()
	reportThroughput(b, b.N)
}

// BenchmarkSDK_Parallel_10: 10 goroutines. Target >= 5,000,000 calls/sec
// aggregate.
func BenchmarkSDK_Parallel_10(b *testing.B) {
	l := benchLogger(b)
	b.ReportAllocs()
	runParallel(b, 10, func() {
		l.Info("load test message", String("k", "v"))
	})
}

// BenchmarkSDK_Parallel_100: 100 goroutines. Should not degrade
// proportionally versus _Parallel_10 — a big drop here is the signal that
// channel-send contention (runtime.chansend) has become the bottleneck,
// which is the documented escalation trigger for revisiting the buffered-
// channel choice in ADR-005.
func BenchmarkSDK_Parallel_100(b *testing.B) {
	l := benchLogger(b)
	b.ReportAllocs()
	runParallel(b, 100, func() {
		l.Info("load test message", String("k", "v"))
	})
}

// BenchmarkSDK_WithFields_5: 5 mixed-type fields per call. Must remain
// 0 allocs/op — the escape hatch types (Any/Err) are excluded by design.
func BenchmarkSDK_WithFields_5(b *testing.B) {
	l := benchLogger(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("load test message",
			String("user_id", "u_123"),
			Int("item_count", 3),
			Float64("total", 99.95),
			Bool("cache_hit", true),
			Duration("elapsed", 1500*time.Millisecond),
		)
	}
	b.StopTimer()
	reportThroughput(b, b.N)
}

// BenchmarkSDK_WithObject_Nested: a 3-level nested Object field. Expected
// 1-2 allocs/op — unavoidable for nested serialisation (child-field slice
// boxing into Field.Interface, ADR-005), unlike the flat scalar paths.
func BenchmarkSDK_WithObject_Nested(b *testing.B) {
	l := benchLogger(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("load test message",
			Object("user",
				String("id", "u_123"),
				Object("permissions",
					Bool("billing", true),
					Bool("admin", false),
				),
			),
		)
	}
	b.StopTimer()
	reportThroughput(b, b.N)
}
