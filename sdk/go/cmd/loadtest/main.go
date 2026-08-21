// Command loadtest is a standalone (non-benchmark) sustained-load driver
// for the OpenTrace Go SDK. Unlike the go test -bench suite (which measures
// steady-state per-call cost), this program runs for minutes against an
// embedded mock collector, sampling throughput, buffer utilisation, drop
// rate, and process memory every 10 seconds — the shape of measurement
// needed to find breaking points and memory-leak-shaped growth curves that
// a sub-second benchmark cannot reveal.
//
// Examples (from sdk/go/):
//
//	go run ./cmd/loadtest -rps 50000 -duration 2m
//	go run ./cmd/loadtest -rps 100000 -duration 10m -mock-latency 100ms
//	GOGC=400 go run ./cmd/loadtest -rps 100000 -duration 2m   # scenario C, see the runbook
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"time"

	opentrace "github.com/opentrace/opentrace-go"
)

func main() {
	var (
		rps         = flag.Int("rps", 10000, "target requests per second, aggregate across all goroutines")
		duration    = flag.Duration("duration", 5*time.Minute, "total run duration")
		mockLatency = flag.Duration("mock-latency", 0, "artificial per-response delay from the embedded mock collector")
		goroutines  = flag.Int("goroutines", 100, "number of concurrent log-emitting goroutines")
	)
	flag.Parse()

	mock := newMockCollector(*mockLatency)
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	logger, err := opentrace.New(
		opentrace.WithCollectorEndpoint(srv.URL),
		opentrace.WithServiceName("loadtest"),
		opentrace.WithBatchSize(500),
		opentrace.WithBatchInterval(500*time.Millisecond),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "opentrace init failed:", err)
		os.Exit(1)
	}

	fmt.Printf("loadtest: target=%d rps, duration=%s, goroutines=%d, mock-latency=%s, GOGC=%s\n",
		*rps, *duration, *goroutines, *mockLatency, envOr("GOGC", "100 (default)"))

	var generated atomic.Int64
	stop := make(chan struct{})
	var wg sync.WaitGroup

	perGoroutineRPS := *rps / *goroutines
	if perGoroutineRPS < 1 {
		perGoroutineRPS = 1
	}
	interval := time.Second / time.Duration(perGoroutineRPS)

	for g := 0; g < *goroutines; g++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					logger.Info("loadtest event",
						opentrace.Int("worker", workerID),
						opentrace.Float64("value", rand.Float64()*100),
						opentrace.Bool("flag", rand.IntN(2) == 0),
					)
					generated.Add(1)
				}
			}
		}(g)
	}

	report := sampleAndReport(logger, mock, &generated, *duration)

	close(stop)
	wg.Wait()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := logger.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintln(os.Stderr, "shutdown incomplete:", err)
	}

	report.print(logger, mock)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
