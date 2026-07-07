package opentrace

import "github.com/opentrace/opentrace-go/internal/wire"

// LogEvent is the internal wire type passed from the hot path to the
// background exporter. Defined in internal/wire (ADR-005 D3).
type LogEvent = wire.LogEvent
