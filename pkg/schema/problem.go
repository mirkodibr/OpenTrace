package schema

import (
	"encoding/json"
	"net/http"
)

// Problem is an RFC 7807 Problem Details response body.
type Problem struct {
	Type       string             `json:"type"`
	Title      string             `json:"title"`
	Status     int                `json:"status"`
	Detail     string             `json:"detail,omitempty"`
	Instance   string             `json:"instance,omitempty"`
	Violations []ValidationError  `json:"violations,omitempty"`
}

// ValidationError describes a single schema violation within a request body.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// WriteProblem serialises p as RFC 7807 JSON and writes it to w.
func WriteProblem(w http.ResponseWriter, p Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
