package opentrace

import "time"

// LogEvent is the internal wire type passed from the hot path to the
// background exporter. It is managed via sync.Pool to minimise allocations.
type LogEvent struct {
	Level     Level
	Message   string
	Timestamp time.Time
	Fields    []Field // pre-allocated slice; len reset on release, cap retained
}
