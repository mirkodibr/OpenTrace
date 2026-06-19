package opentrace

import "sync"

// eventPool holds recycled LogEvent structs. The pre-allocated Fields slice
// capacity (16) covers the typical structured log call without growth copies.
var eventPool = sync.Pool{
	New: func() interface{} {
		return &LogEvent{
			Fields: make([]Field, 0, 16),
		}
	},
}

// acquireEvent retrieves a LogEvent from the pool.
func acquireEvent() *LogEvent {
	return eventPool.Get().(*LogEvent)
}

// releaseEvent resets e and returns it to the pool.
// The caller must not access e after calling releaseEvent.
func releaseEvent(e *LogEvent) {
	e.Message = ""
	e.Fields = e.Fields[:0] // retain backing array, zero length
	e.Timestamp = e.Timestamp.Truncate(0) // zero without deallocation
	eventPool.Put(e)
}
