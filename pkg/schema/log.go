package schema

import "time"

// LogLevel represents the severity of a log event.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
)

// Validate returns true if l is a recognised log level.
func (l LogLevel) Validate() bool {
	switch l {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelFatal:
		return true
	}
	return false
}

// LogEvent is the canonical wire-format representation of a single log event.
// It is shared between the collector-service ingest handler and the query-api
// response serializer so both sides stay in sync with the same field names.
type LogEvent struct {
	// Core identity
	ID      string    `json:"id,omitempty"`
	TraceID string    `json:"trace_id,omitempty"`
	SpanID  string    `json:"span_id,omitempty"`

	// Timing
	Timestamp  time.Time `json:"timestamp"`
	ReceivedAt time.Time `json:"received_at,omitempty"`

	// Classification
	ServiceName  string   `json:"service_name"`
	Severity     LogLevel `json:"severity"`
	SeverityText string   `json:"severity_text,omitempty"`

	// Content
	Body      string `json:"body"`
	SchemaURL string `json:"schema_url,omitempty"`

	// Structured metadata (OTel resource + event attributes)
	ResourceAttributes map[string]any `json:"resource_attributes,omitempty"`
	LogAttributes      map[string]any `json:"log_attributes,omitempty"`
}

// IngestLogsRequest is the payload accepted by POST /api/v1/logs.
type IngestLogsRequest struct {
	Events []LogEvent `json:"events"`
}

// IngestLogsResponse is the 202 Accepted response from POST /api/v1/logs.
type IngestLogsResponse struct {
	BatchID               string `json:"batch_id"`
	Accepted              int    `json:"accepted"`
	EstimatedProcessingMs int    `json:"estimated_processing_ms"`
}
