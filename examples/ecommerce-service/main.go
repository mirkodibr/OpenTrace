// Command ecommerce-service is the OpenTrace SDK reference integration: a
// realistic mock shop that logs structured events through the SDK across
// checkout, catalogue, and order flows, with an optional in-process load
// generator.
//
// Run against a local OpenTrace stack (docker compose up):
//
//	go run ./examples/ecommerce-service                 # server only, port 9090
//	go run ./examples/ecommerce-service -rps 50 -duration 2m   # with load
//
// Then watch events arrive in the dashboard (http://localhost:3000) or via:
//
//	curl "http://localhost:8081/api/v1/logs?service_name=ecommerce-service&start_time=...&end_time=..."
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi/v5"
	opentrace "github.com/opentrace/opentrace-go"

	"github.com/opentrace/opentrace/examples/ecommerce-service/handler"
	"github.com/opentrace/opentrace/examples/ecommerce-service/loadgen"
	"github.com/opentrace/opentrace/examples/ecommerce-service/middleware"
	"github.com/opentrace/opentrace/examples/ecommerce-service/service"
)

func main() {
	var (
		addr      = flag.String("addr", ":9090", "listen address")
		collector = flag.String("collector", envOr("OPENTRACE_COLLECTOR_ENDPOINT", "http://localhost:8080"), "collector endpoint")
		rps       = flag.Int("rps", 0, "in-process load generator target RPS (0 = disabled)")
		duration  = flag.Duration("duration", time.Minute, "load generator run time")
	)
	flag.Parse()

	logger, err := opentrace.New(
		opentrace.WithCollectorEndpoint(*collector),
		opentrace.WithServiceName("ecommerce-service"),
		opentrace.WithServiceVersion("1.0.0"),
		opentrace.WithEnvironment("development"),
		opentrace.WithMinLevel(opentrace.LevelDebug),
		opentrace.WithBatchSize(250),
		opentrace.WithBatchInterval(time.Second),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "opentrace init failed:", err)
		os.Exit(1)
	}

	r := chi.NewRouter()
	r.Use(middleware.Tracing(logger))
	inventory := service.Inventory{}
	payments := service.Payments{}
	r.Post("/checkout", handler.Checkout(inventory, payments))
	r.Get("/products", handler.Products())
	r.Get("/orders/{orderID}", handler.Orders())

	srv := &http.Server{
		Addr:              *addr,
		Handler:           r,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		logger.Info("ecommerce-service listening", opentrace.String("addr", *addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", opentrace.Err(err))
			stop()
		}
	}()

	if *rps > 0 {
		go func() {
			// Give the server a beat to come up, then drive traffic.
			time.Sleep(300 * time.Millisecond)
			stats := loadgen.Run(ctx, loadgen.Config{
				BaseURL:  "http://localhost" + *addr,
				RPS:      *rps,
				Duration: *duration,
			})
			logger.Info("load generation finished",
				opentrace.Int64("requests", stats.Requests),
				opentrace.Int64("errors", stats.Errors),
				opentrace.Float64("achieved_rps", stats.AchievedRPS),
				opentrace.Duration("p95", stats.P95),
			)
			stop() // load run complete — shut the demo down
		}()
	}

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	if err := logger.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintln(os.Stderr, "logger shutdown incomplete:", err)
	}
	fmt.Printf("shut down cleanly — %d events dropped\n", logger.DroppedCount())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
