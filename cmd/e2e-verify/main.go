// Command e2e-verify proves the complete OpenTrace pipeline end to end:
// SDK → collector-service → PostgreSQL → query-api. It generates uniquely
// tagged events through the real SDK, polls the query API until they become
// visible, and verifies count, field integrity, and timing.
//
// Run against a local stack (docker compose up):
//
//	go run ./cmd/e2e-verify
//	go run ./cmd/e2e-verify -events 250 -no-compression
//
// Exit code 0 = all checks passed; 1 = verification failed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	opentrace "github.com/opentrace/opentrace-go"
)

type queryResponse struct {
	Data []struct {
		Body          string         `json:"body"`
		ServiceName   string         `json:"service_name"`
		Severity      string         `json:"severity"`
		LogAttributes map[string]any `json:"log_attributes"`
	} `json:"data"`
	NextCursor *string `json:"next_cursor"`
}

func main() {
	var (
		collector     = flag.String("collector", envOr("OPENTRACE_COLLECTOR_ENDPOINT", "http://localhost:8080"), "collector endpoint")
		queryAPI      = flag.String("query-api", envOr("OPENTRACE_QUERY_API", "http://localhost:8081"), "query-api endpoint")
		eventCount    = flag.Int("events", 100, "number of events to send")
		waitBudget    = flag.Duration("wait", 15*time.Second, "max time to wait for events to become queryable")
		noCompression = flag.Bool("no-compression", false, "disable SDK gzip (regression check for uncompressed path)")
	)
	flag.Parse()

	uniqueID := fmt.Sprintf("e2e-test-%d", time.Now().UnixNano())
	fmt.Printf("→ unique marker: %s\n", uniqueID)

	// STEP 1 — GENERATE through the real SDK.
	logger, err := opentrace.New(
		opentrace.WithCollectorEndpoint(*collector),
		opentrace.WithServiceName("e2e-verify"),
		opentrace.WithServiceVersion(opentrace.Version),
		opentrace.WithEnvironment("e2e"),
		opentrace.WithBatchSize(50),
		opentrace.WithBatchInterval(500*time.Millisecond),
		opentrace.WithCompression(!*noCompression),
	)
	if err != nil {
		fail("SDK init: %v", err)
	}

	genStart := time.Now()
	for i := 0; i < *eventCount; i++ {
		logger.Info("e2e verification event "+uniqueID,
			opentrace.String("e2e_id", uniqueID),
			opentrace.Int("sequence", i),
			opentrace.Bool("verification", true),
		)
	}
	genElapsed := time.Since(genStart)
	pass("event generation: %d events in %s (%.0f ns/event)",
		*eventCount, genElapsed, float64(genElapsed.Nanoseconds())/float64(*eventCount))

	// STEP 2 — FLUSH: shutdown drains every buffered event.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	flushStart := time.Now()
	if err := logger.Shutdown(shutdownCtx); err != nil {
		fail("SDK shutdown: %v", err)
	}
	if n := logger.DroppedCount(); n > 0 {
		fail("SDK dropped %d events during generation/flush", n)
	}
	pass("flush complete in %s, 0 events dropped", time.Since(flushStart))

	// STEP 3 — QUERY: poll until all events are visible or budget expires.
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(*waitBudget)
	var (
		found    int
		lastResp queryResponse
		waited   time.Duration
	)
	pollStart := time.Now()
	for {
		found, lastResp = countEvents(client, *queryAPI, uniqueID)
		waited = time.Since(pollStart)
		if found >= *eventCount || time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if found != *eventCount {
		fail("ingest visibility: expected %d events, found %d after %s", *eventCount, found, waited)
	}
	pass("ingest latency: %d/%d events queryable after %s", found, *eventCount, waited)

	// STEP 4 — VERIFY field integrity on a sample row.
	if len(lastResp.Data) == 0 {
		fail("field integrity: query returned no rows to inspect")
	}
	sample := lastResp.Data[0]
	switch {
	case sample.ServiceName != "e2e-verify":
		fail("field integrity: service_name = %q", sample.ServiceName)
	case sample.Severity != "info":
		fail("field integrity: severity = %q", sample.Severity)
	case sample.LogAttributes["e2e_id"] != uniqueID:
		fail("field integrity: log_attributes.e2e_id = %v", sample.LogAttributes["e2e_id"])
	case sample.LogAttributes["verification"] != true:
		fail("field integrity: log_attributes.verification = %v", sample.LogAttributes["verification"])
	}
	pass("field integrity: attributes preserved end to end")

	fmt.Println("\nALL CHECKS PASSED")
}

// countEvents pages through the query API counting events tagged uniqueID.
func countEvents(client *http.Client, queryAPI, uniqueID string) (int, queryResponse) {
	total := 0
	var last queryResponse
	cursor := ""
	for {
		q := url.Values{}
		q.Set("keyword", uniqueID)
		q.Set("service_name", "e2e-verify")
		q.Set("start_time", time.Now().Add(-10*time.Minute).UTC().Format(time.RFC3339))
		q.Set("end_time", time.Now().Add(time.Minute).UTC().Format(time.RFC3339))
		q.Set("limit", "1000")
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		resp, err := client.Get(queryAPI + "/api/v1/logs?" + q.Encode())
		if err != nil {
			return total, last
		}
		var qr queryResponse
		err = json.NewDecoder(resp.Body).Decode(&qr)
		resp.Body.Close()
		if err != nil {
			return total, last
		}
		total += len(qr.Data)
		last = qr
		if qr.NextCursor == nil || *qr.NextCursor == "" || len(qr.Data) == 0 {
			return total, last
		}
		cursor = *qr.NextCursor
	}
}

func pass(format string, args ...any) {
	fmt.Printf("✓ "+format+"\n", args...)
}

func fail(format string, args ...any) {
	fmt.Printf("✗ FAILED: "+format+"\n", args...)
	os.Exit(1)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
