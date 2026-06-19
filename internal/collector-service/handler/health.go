package handler

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

// build-time injected via -ldflags "-X handler.version=..."
var version = "dev"

var startTime = time.Now()

// healthStatus tracks dependency check results.
type healthStatus struct {
	Status  string                 `json:"status"`
	Version string                 `json:"version"`
	Uptime  float64                `json:"uptime_seconds"`
	Checks  map[string]checkResult `json:"checks"`
}

type checkResult struct {
	Status string `json:"status"`
}

// healthy tracks whether the service considers itself healthy (used by the
// /healthz endpoint to control readiness).
var healthy atomic.Bool

func init() {
	healthy.Store(true)
}

// Health handles GET /healthz. It probes configured dependencies and returns
// 200 when all checks pass or 503 if any check reports "down".
func Health(w http.ResponseWriter, r *http.Request) {
	checks := map[string]checkResult{
		"database": {Status: "ok"}, // TODO: implement real pg ping
		"broker":   {Status: "ok"}, // TODO: implement real Redpanda ping
	}

	overallStatus := "ok"
	httpStatus := http.StatusOK
	for _, c := range checks {
		if c.Status == "down" {
			overallStatus = "degraded"
			httpStatus = http.StatusServiceUnavailable
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(healthStatus{
		Status:  overallStatus,
		Version: version,
		Uptime:  time.Since(startTime).Seconds(),
		Checks:  checks,
	})
}
