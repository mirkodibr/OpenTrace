// Package loadgen drives realistic traffic against the example service from
// inside the same process: 60% checkout, 30% products, 10% orders, at a
// configurable request rate.
package loadgen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Config parameterises a load generation run.
type Config struct {
	BaseURL  string        // e.g. http://localhost:9090
	RPS      int           // target requests per second
	Duration time.Duration // total run time
}

// Stats summarises a completed run.
type Stats struct {
	Requests    int64
	Errors      int64
	AchievedRPS float64
	P95         time.Duration
}

var (
	products   = []string{"p-1001", "p-1002", "p-1003", "p-1004", "p-1005", "p-1006", "p-1007", "p-1008", "p-1009", "p-1010"}
	categories = []string{"electronics", "books", "garden", "toys", "sports"}
)

// Run generates load until cfg.Duration elapses or ctx is cancelled, then
// returns the aggregated stats.
func Run(ctx context.Context, cfg Config) Stats {
	client := &http.Client{Timeout: 5 * time.Second}
	interval := time.Second / time.Duration(max(cfg.RPS, 1))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	deadline := time.After(cfg.Duration)

	var (
		requests atomic.Int64
		errs     atomic.Int64
		mu       sync.Mutex
		lats     []time.Duration
		wg       sync.WaitGroup
	)
	start := time.Now()

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-deadline:
			break loop
		case <-ticker.C:
			wg.Add(1)
			go func() {
				defer wg.Done()
				t0 := time.Now()
				err := fire(ctx, client, cfg.BaseURL)
				lat := time.Since(t0)
				requests.Add(1)
				if err != nil {
					errs.Add(1)
				}
				mu.Lock()
				lats = append(lats, lat)
				mu.Unlock()
			}()
		}
	}
	wg.Wait()

	elapsed := time.Since(start)
	stats := Stats{
		Requests:    requests.Load(),
		Errors:      errs.Load(),
		AchievedRPS: float64(requests.Load()) / elapsed.Seconds(),
	}
	mu.Lock()
	if len(lats) > 0 {
		sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
		stats.P95 = lats[len(lats)*95/100]
	}
	mu.Unlock()
	return stats
}

// fire sends one request with the 60/30/10 endpoint mix.
func fire(ctx context.Context, client *http.Client, baseURL string) error {
	switch n := rand.N(100); {
	case n < 60:
		return checkout(ctx, client, baseURL)
	case n < 90:
		return getProducts(ctx, client, baseURL)
	default:
		return getOrder(ctx, client, baseURL)
	}
}

func checkout(ctx context.Context, client *http.Client, baseURL string) error {
	body := map[string]any{
		"user_id": fmt.Sprintf("u_%04d", rand.N(100)),
		"items": []map[string]any{
			{"product_id": products[rand.N(len(products))], "quantity": 1 + rand.N(3)},
		},
	}
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/checkout", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	return do(client, req, http.StatusCreated, http.StatusBadGateway)
}

func getProducts(ctx context.Context, client *http.Client, baseURL string) error {
	url := fmt.Sprintf("%s/products?category=%s", baseURL, categories[rand.N(len(categories))])
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	return do(client, req, http.StatusOK)
}

func getOrder(ctx context.Context, client *http.Client, baseURL string) error {
	// ~15% deliberately invalid IDs to exercise the not-found ERROR path.
	id := fmt.Sprintf("ord_%04d", rand.N(1000))
	if rand.N(100) < 15 {
		id = fmt.Sprintf("bogus_%04d", rand.N(1000))
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/orders/"+id, nil)
	return do(client, req, http.StatusOK, http.StatusNotFound)
}

// do executes the request and treats any status outside expected as an error.
func do(client *http.Client, req *http.Request, expected ...int) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	for _, code := range expected {
		if resp.StatusCode == code {
			return nil
		}
	}
	return fmt.Errorf("unexpected status %d for %s", resp.StatusCode, req.URL.Path)
}
