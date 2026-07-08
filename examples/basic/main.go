// Command basic demonstrates the three ways to configure the OpenTrace Go
// SDK. Run it against a local stack (docker compose up) with:
//
//	go run ./examples/basic
//
// It emits a handful of structured events and shuts down cleanly; open the
// dashboard (http://localhost:3000) or query the API to see them arrive.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	opentrace "github.com/opentrace/opentrace-go"
)

func main() {
	logger, err := newLogger()
	if err != nil {
		fmt.Fprintln(os.Stderr, "opentrace init failed:", err)
		os.Exit(1)
	}

	// Structured events with typed, zero-allocation fields.
	logger.Info("service started",
		opentrace.String("region", "eu-west-1"),
		opentrace.Int("replicas", 3),
	)
	logger.Warn("cache miss ratio elevated",
		opentrace.Float64("miss_ratio", 0.37),
		opentrace.Bool("degraded", false),
	)
	logger.Error("payment provider unreachable",
		opentrace.Err(errors.New("dial tcp 10.0.0.7:443: connection refused")),
		opentrace.Duration("timeout_after", 3*time.Second),
	)

	// Child loggers pre-set fields for a whole scope.
	reqLogger := logger.With(
		opentrace.String("request_id", "req-2f9c"),
		opentrace.String("user_id", "u_1042"),
	)
	reqLogger.Info("checkout completed", opentrace.Float64("total", 129.99))

	// Trace correlation: store IDs on the context once, derive loggers
	// anywhere below. (OpenTelemetry users bridge with one line — see
	// opentrace.ContextWithTrace docs.)
	ctx := opentrace.ContextWithTrace(context.Background(),
		"0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331")
	logger.WithContext(ctx).Info("traced operation finished")

	// Always shut down: flushes buffered events within the deadline.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := logger.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintln(os.Stderr, "shutdown incomplete:", err)
	}
	fmt.Printf("done — %d events dropped\n", logger.DroppedCount())
}

// newLogger shows the three configuration styles. Pick ONE in real code.
func newLogger() (*opentrace.Logger, error) {
	switch os.Getenv("CONFIG_STYLE") {

	case "env":
		// STYLE 2 — fully environment-driven. Deployment config (Helm,
		// compose, systemd) owns every setting; code stays generic:
		//
		//	OPENTRACE_COLLECTOR_ENDPOINT=http://localhost:8080
		//	OPENTRACE_SERVICE_NAME=basic-example
		//	OPENTRACE_MIN_LEVEL=info
		return opentrace.New()

	case "mixed":
		// STYLE 3 — mixed. Secrets and endpoints come from the
		// environment; app-specific tuning stays in code where it is
		// reviewed with the service. Programmatic options always win
		// over env vars, so the tuning below cannot be overridden by a
		// stray variable.
		return opentrace.New(
			opentrace.WithServiceName("basic-example"),
			opentrace.WithBatchSize(250),
			opentrace.WithBatchInterval(time.Second),
			opentrace.WithHeader("X-API-Key", os.Getenv("OPENTRACE_API_KEY")),
		)

	default:
		// STYLE 1 — fully programmatic. Best for quick starts and tests:
		// everything is explicit and greppable.
		endpoint := os.Getenv("OPENTRACE_COLLECTOR_ENDPOINT")
		if endpoint == "" {
			endpoint = "http://localhost:8080"
		}
		return opentrace.New(
			opentrace.WithCollectorEndpoint(endpoint),
			opentrace.WithServiceName("basic-example"),
			opentrace.WithServiceVersion("1.0.0"),
			opentrace.WithEnvironment("development"),
			opentrace.WithMinLevel(opentrace.LevelDebug),
			opentrace.WithDebug(true),
		)
	}
}
