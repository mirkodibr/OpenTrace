package exporter

import (
	"bytes"
	"testing"
	"time"

	"github.com/opentrace/opentrace-go/internal/wire"
)

// realisticBatch builds a 100-event batch resembling the e-commerce example
// service's checkout log line: a handful of scalar fields per event,
// matching the field mix real callers exercise.
func realisticBatch(n int) []*wire.LogEvent {
	batch := make([]*wire.LogEvent, n)
	for i := 0; i < n; i++ {
		e := wire.AcquireEvent()
		e.Level = wire.LevelInfo
		e.Message = "checkout completed"
		e.Timestamp = time.Now()
		e.Fields = append(e.Fields,
			wire.String("user_id", "u_1042"),
			wire.String("order_id", "ord_2f9c8a1b"),
			wire.Float64("total_amount", 129.99),
			wire.Int("item_count", 3),
			wire.Duration("total_duration", 42*time.Millisecond),
		)
		batch[i] = e
	}
	return batch
}

var benchRes = wire.Resource{
	ServiceName: "bench-service", ServiceVersion: "1.0.0",
	Environment: "bench", Hostname: "bench-host", PID: 1,
}

// Both benchmarks build the batch ONCE outside the timed loop, so b.N
// iterations measure only serialisation cost — event/field construction
// (already covered by the SDK hot-path benchmarks) would otherwise pollute
// the allocation count being compared here.

func BenchmarkSerializeJSON_100Events(b *testing.B) {
	batch := realisticBatch(100)
	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := serializeJSON(&buf, batch, benchRes); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSerializeWritten_100Events(b *testing.B) {
	batch := realisticBatch(100)
	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := serialize(&buf, batch, benchRes); err != nil {
			b.Fatal(err)
		}
	}
}
