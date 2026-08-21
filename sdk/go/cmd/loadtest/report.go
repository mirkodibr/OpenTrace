package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	opentrace "github.com/opentrace/opentrace-go"
)

// mockCollector is an embedded stand-in for the OpenTrace collector: it
// decompresses (if needed), counts events, and can simulate response
// latency to model a slow/overloaded backend (Scenario A in the runbook).
type mockCollector struct {
	delay    time.Duration
	received atomic.Int64
	requests atomic.Int64
}

func newMockCollector(delay time.Duration) *mockCollector {
	return &mockCollector{delay: delay}
}

func (m *mockCollector) handler() http.HandlerFunc {
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
		m.received.Add(int64(len(payload.Events)))
		m.requests.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}
}

// sample is one 10-second measurement window.
type sample struct {
	t              time.Time
	generatedRPS   float64
	bufferUtilPct  float64
	droppedTotal   int64
	heapAllocBytes uint64
	numGoroutine   int
	numGC          uint32
	gcPauseTotalNs uint64
}

// runReport accumulates samples over the run for the final summary.
type runReport struct {
	mu      sync.Mutex
	samples []sample
	startAt time.Time
	startGC runtime.MemStats
}

// sampleAndReport blocks for duration, printing one line every 10 seconds
// and recording it for the final report.
func sampleAndReport(logger *opentrace.Logger, mock *mockCollector, generated *atomic.Int64, duration time.Duration) *runReport {
	r := &runReport{startAt: time.Now()}
	runtime.ReadMemStats(&r.startGC)

	const window = 10 * time.Second
	ticker := time.NewTicker(window)
	defer ticker.Stop()

	deadline := time.Now().Add(duration)
	var lastGenerated int64
	fmt.Println("\ntime      gen_rps   buf_util%   dropped   heap_MB   goroutines   numGC   gc_pause_ms")

	for now := range ticker.C {
		g := generated.Load()
		s := sample{
			t:             now,
			generatedRPS:  float64(g-lastGenerated) / window.Seconds(),
			bufferUtilPct: logger.BufferUtilization() * 100,
			droppedTotal:  logger.DroppedCount(),
			numGoroutine:  runtime.NumGoroutine(),
		}
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		s.heapAllocBytes = ms.HeapAlloc
		s.numGC = ms.NumGC
		s.gcPauseTotalNs = ms.PauseTotalNs
		lastGenerated = g

		r.mu.Lock()
		r.samples = append(r.samples, s)
		r.mu.Unlock()

		fmt.Printf("%-9s %9.0f %10.1f %9d %9.1f %12d %7d %13.1f\n",
			time.Since(r.startAt).Round(time.Second),
			s.generatedRPS, s.bufferUtilPct, s.droppedTotal,
			float64(s.heapAllocBytes)/(1<<20), s.numGoroutine, s.numGC,
			float64(s.gcPauseTotalNs)/1e6)

		if time.Now().After(deadline) {
			return r
		}
	}
	return r
}

// print writes the final summary: peak/mean throughput, total dropped,
// and memory growth over the run.
func (r *runReport) print(logger *opentrace.Logger, mock *mockCollector) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var endGC runtime.MemStats
	runtime.ReadMemStats(&endGC)

	fmt.Println("\n=== Load Test Summary ===")
	fmt.Printf("duration:          %s\n", time.Since(r.startAt).Round(time.Second))
	fmt.Printf("events received:   %d (mock collector)\n", mock.received.Load())
	fmt.Printf("requests received: %d (mock collector)\n", mock.requests.Load())
	fmt.Printf("events dropped:    %d (SDK DroppedCount)\n", logger.DroppedCount())

	if len(r.samples) > 0 {
		var peakRPS, sumRPS float64
		for _, s := range r.samples {
			sumRPS += s.generatedRPS
			if s.generatedRPS > peakRPS {
				peakRPS = s.generatedRPS
			}
		}
		fmt.Printf("peak throughput:   %.0f events/sec\n", peakRPS)
		fmt.Printf("mean throughput:   %.0f events/sec\n", sumRPS/float64(len(r.samples)))
	}

	growthMB := float64(int64(endGC.HeapAlloc)-int64(r.startGC.HeapAlloc)) / (1 << 20)
	fmt.Printf("heap growth:       %.1f MB (start %.1f MB -> end %.1f MB)\n",
		growthMB, float64(r.startGC.HeapAlloc)/(1<<20), float64(endGC.HeapAlloc)/(1<<20))
	fmt.Printf("GC cycles:         %d\n", endGC.NumGC-r.startGC.NumGC)
	fmt.Printf("GC pause total:    %.1f ms\n", float64(endGC.PauseTotalNs-r.startGC.PauseTotalNs)/1e6)
}
