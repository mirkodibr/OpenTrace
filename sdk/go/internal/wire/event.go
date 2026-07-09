package wire

import "time"

// LogEvent is the internal wire type passed from the hot path through the
// batcher to the background exporter. It is managed via the pool in this
// package to minimise allocations.
type LogEvent struct {
	Level     Level
	Message   string
	Timestamp time.Time
	Fields    []Field // pre-allocated slice; len reset on release, cap retained
}
