package wire

import (
	"sync"
	"time"
)

// eventPool holds recycled LogEvent structs. The pre-allocated Fields slice
// capacity (16) covers the typical structured log call without growth copies.
var eventPool = sync.Pool{
	New: func() interface{} {
		return &LogEvent{
			Fields: make([]Field, 0, 16),
		}
	},
}

// AcquireEvent retrieves a LogEvent from the pool.
func AcquireEvent() *LogEvent {
	return eventPool.Get().(*LogEvent)
}

// ReleaseEvent resets e and returns it to the pool.
//
// Ownership rule (ADR-005 D2): a *LogEvent has exactly one owner at any time —
// the hot path until enqueue, the buffer/batcher after enqueue, the exporter
// after flush. Only the current owner may call ReleaseEvent, and the caller
// must not access e afterwards. The exporter releases events immediately after
// serialisation, before any network I/O.
func ReleaseEvent(e *LogEvent) {
	e.Level = 0
	e.Message = ""
	// Zero the used field slots so string/interface references from the
	// previous call do not outlive it (data-leak prevention), then reset
	// length while retaining the backing array.
	clear(e.Fields)
	e.Fields = e.Fields[:0]
	e.Timestamp = time.Time{}
	eventPool.Put(e)
}
