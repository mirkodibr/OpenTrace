// seed sends 1,000 sample log events to the collector-service for local testing.
// Usage: go run ./scripts/seed/main.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
)

var (
	services  = []string{"payment-service", "checkout-service", "inventory-service", "auth-service", "notification-service"}
	levels    = []string{"debug", "info", "warn", "error"}
	messages  = []string{
		"User signed in successfully",
		"Payment processed",
		"Inventory check completed",
		"Database query executed",
		"Cache miss — falling back to database",
		"HTTP request completed",
		"Background job started",
		"Configuration reloaded",
		"Rate limit threshold reached",
		"Circuit breaker opened",
	}
)

type logEvent struct {
	Timestamp    string         `json:"timestamp"`
	ServiceName  string         `json:"service_name"`
	Severity     string         `json:"severity"`
	Body         string         `json:"body"`
	LogAttributes map[string]any `json:"log_attributes,omitempty"`
}

type ingestRequest struct {
	Events []logEvent `json:"events"`
}

func main() {
	endpoint := os.Getenv("COLLECTOR_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8080"
	}

	total := 1000
	batchSize := 100
	sent := 0

	log.Printf("Seeding %d events to %s in batches of %d", total, endpoint, batchSize)

	for sent < total {
		batch := make([]logEvent, 0, batchSize)
		for i := 0; i < batchSize && sent+i < total; i++ {
			batch = append(batch, logEvent{
				Timestamp:   time.Now().Add(-time.Duration(rand.Intn(3600)) * time.Second).UTC().Format(time.RFC3339Nano),
				ServiceName: services[rand.Intn(len(services))],
				Severity:    levels[rand.Intn(len(levels))],
				Body:        messages[rand.Intn(len(messages))],
				LogAttributes: map[string]any{
					"request_id": fmt.Sprintf("req_%d", rand.Intn(1_000_000)),
					"user_id":    fmt.Sprintf("u_%d", rand.Intn(10_000)),
					"duration_ms": rand.Intn(500),
				},
			})
		}

		body, _ := json.Marshal(ingestRequest{Events: batch})
		resp, err := http.Post(endpoint+"/api/v1/logs", "application/json", bytes.NewReader(body))
		if err != nil {
			log.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			log.Fatalf("unexpected status: %d", resp.StatusCode)
		}

		sent += len(batch)
		log.Printf("Sent %d / %d events", sent, total)
	}

	log.Printf("Done. %d events sent successfully.", sent)
}
