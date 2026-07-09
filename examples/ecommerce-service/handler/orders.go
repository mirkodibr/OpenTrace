package handler

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	opentrace "github.com/opentrace/opentrace-go"

	"github.com/opentrace/opentrace/examples/ecommerce-service/middleware"
)

// Orders simulates an order lookup (15ms) where invalid IDs (those not
// prefixed "ord_") are not found — the load generator sends ~15% of these
// to exercise the ERROR path.
func Orders() http.HandlerFunc {
	statuses := []string{"pending", "paid", "shipped", "delivered"}
	return func(w http.ResponseWriter, r *http.Request) {
		log := middleware.LoggerFromContext(r.Context())
		orderID := chi.URLParam(r, "orderID")
		time.Sleep(15 * time.Millisecond) // simulated lookup latency

		if !strings.HasPrefix(orderID, "ord_") {
			log.Error("order not found",
				opentrace.String("order_id", orderID),
				opentrace.Int("http.status_code", http.StatusNotFound),
			)
			http.Error(w, "order not found", http.StatusNotFound)
			return
		}

		status := statuses[rand.N(len(statuses))]
		itemCount := 1 + rand.N(5)
		log.Info("order retrieved",
			opentrace.String("order_id", orderID),
			opentrace.String("status", status),
			opentrace.Int("item_count", itemCount),
		)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"order_id": orderID, "status": status, "item_count": itemCount,
		})
	}
}
