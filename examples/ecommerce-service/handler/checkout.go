// Package handler implements the example e-commerce endpoints. Every
// handler pulls its request-scoped logger from the context (set by the
// tracing middleware) so all events carry request_id automatically.
package handler

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/google/uuid"
	opentrace "github.com/opentrace/opentrace-go"

	"github.com/opentrace/opentrace/examples/ecommerce-service/middleware"
	"github.com/opentrace/opentrace/examples/ecommerce-service/service"
)

type checkoutRequest struct {
	UserID string `json:"user_id"`
	Items  []struct {
		ProductID string `json:"product_id"`
		Quantity  int    `json:"quantity"`
	} `json:"items"`
}

// Checkout simulates inventory check → payment → order creation and
// demonstrates DEBUG step events, WARN business anomalies, ERROR failures
// with error fields, and a With()-scoped payment logger.
func Checkout(inventory service.Inventory, payments service.Payments) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := middleware.LoggerFromContext(r.Context())
		start := time.Now()

		var req checkoutRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" || len(req.Items) == 0 {
			log.Warn("invalid checkout payload", opentrace.Err(err))
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}

		// Step 1 — inventory.
		itemCount := 0
		var invElapsed time.Duration
		for _, item := range req.Items {
			res := inventory.Check(item.ProductID, item.Quantity)
			itemCount += item.Quantity
			invElapsed += res.Elapsed
			if res.LowStock {
				log.Warn("inventory low",
					opentrace.String("product_id", item.ProductID),
					opentrace.Int("quantity_requested", item.Quantity),
					opentrace.Bool("requires_review", true),
				)
			}
		}
		log.Debug("inventory_checked",
			opentrace.Int("item_count", itemCount),
			opentrace.Duration("db_query_time", invElapsed),
		)

		// Step 2 — payment, with a payment-scoped child logger.
		total := float64(itemCount) * (10 + rand.Float64()*90)
		tax := total * 0.19
		payLog := log.With(opentrace.String("payment_method", "card"))
		payLog.Debug("payment_initiated", opentrace.Float64("total_amount", total))
		charge, err := payments.Charge(req.UserID, total)
		if err != nil {
			payLog.Error("payment failed",
				opentrace.Err(err),
				opentrace.String("user_id", req.UserID),
				opentrace.Float64("total_amount", total),
				opentrace.Duration("payment_processing_time", charge.Elapsed),
				opentrace.Bool("is_retry", false),
			)
			http.Error(w, "payment failed", http.StatusBadGateway)
			return
		}
		payLog = payLog.With(opentrace.String("payment_id", charge.PaymentID))
		payLog.Debug("payment_captured",
			opentrace.Duration("payment_processing_time", charge.Elapsed),
		)

		// Step 3 — order creation.
		orderID := "ord_" + uuid.NewString()[:8]
		log.Debug("order_created", opentrace.String("order_id", orderID))

		log.Info("checkout completed",
			opentrace.String("user_id", req.UserID),
			opentrace.String("order_id", orderID),
			opentrace.Float64("total_amount", total),
			opentrace.Float64("tax_amount", tax),
			opentrace.Int("item_count", itemCount),
			opentrace.Duration("total_duration", time.Since(start)),
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"order_id": orderID, "payment_id": charge.PaymentID, "total": total,
		})
	}
}
