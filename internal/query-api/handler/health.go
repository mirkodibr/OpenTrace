package handler

import (
	"encoding/json"
	"net/http"
	"time"
)

var version = "dev"
var startTime = time.Now()

type healthStatus struct {
	Status  string                 `json:"status"`
	Version string                 `json:"version"`
	Uptime  float64                `json:"uptime_seconds"`
	Checks  map[string]checkResult `json:"checks"`
}

type checkResult struct {
	Status string `json:"status"`
}

// Health handles GET /healthz.
func Health(w http.ResponseWriter, r *http.Request) {
	checks := map[string]checkResult{
		"database":   {Status: "ok"}, // TODO: implement real pg ping
		"clickhouse": {Status: "ok"}, // TODO: implement real CH ping
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
