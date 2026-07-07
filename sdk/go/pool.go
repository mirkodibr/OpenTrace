package opentrace

import "github.com/opentrace/opentrace-go/internal/wire"

// acquireEvent and releaseEvent forward to the shared pool in internal/wire.
// See wire.ReleaseEvent for the single-ownership rule (ADR-005 D2).
func acquireEvent() *LogEvent  { return wire.AcquireEvent() }
func releaseEvent(e *LogEvent) { wire.ReleaseEvent(e) }
