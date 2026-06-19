package opentrace

// eventBuffer is a fixed-capacity, non-blocking channel-based queue that
// bridges the hot-path callers and the background exporter goroutine.
//
// Drop policy: when the channel is full, tryEnqueue returns false and the
// caller increments the dropped counter. Blocking is never acceptable on a
// logger hot path — a 1 µs stall in a high-RPS web handler adds directly to
// request latency.
type eventBuffer struct {
	ch chan *LogEvent
}

func newEventBuffer(capacity int) *eventBuffer {
	return &eventBuffer{ch: make(chan *LogEvent, capacity)}
}

// tryEnqueue attempts a non-blocking send. Returns false when the buffer is full.
func (b *eventBuffer) tryEnqueue(evt *LogEvent) bool {
	select {
	case b.ch <- evt:
		return true
	default:
		return false
	}
}
