package handler

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"time"

	opentrace "github.com/opentrace/opentrace-go"

	"github.com/opentrace/opentrace/examples/ecommerce-service/middleware"
)

// Products simulates a category listing with a cache in front of a database
// (20ms simulated DB latency, ~70% cache hit rate).
func Products() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := middleware.LoggerFromContext(r.Context())
		start := time.Now()
		category := r.URL.Query().Get("category")

		cacheHit := rand.N(100) < 70
		if !cacheHit {
			time.Sleep(20 * time.Millisecond) // simulated database query
		}
		resultCount := rand.N(12) // 0 hits sometimes, to exercise the WARN path

		if resultCount == 0 {
			log.Warn("no products found for category",
				opentrace.String("category", category),
				opentrace.Bool("cache_hit", cacheHit),
			)
		} else {
			log.Info("products listed",
				opentrace.String("category", category),
				opentrace.Int("result_count", resultCount),
				opentrace.Bool("cache_hit", cacheHit),
				opentrace.Duration("duration", time.Since(start)),
			)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"category": category, "count": resultCount, "cache_hit": cacheHit,
		})
	}
}
