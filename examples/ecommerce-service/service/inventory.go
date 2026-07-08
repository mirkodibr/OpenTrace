// Package service simulates the business dependencies of the example
// e-commerce service with realistic latencies and failure rates.
package service

import (
	"math/rand/v2"
	"time"
)

// Inventory simulates a stock-keeping backend.
type Inventory struct{}

// CheckResult describes a simulated inventory lookup.
type CheckResult struct {
	InStock  bool
	LowStock bool          // ~10% of checks: stock below reorder threshold
	Elapsed  time.Duration // simulated backend latency
}

// Check simulates an inventory availability lookup (5-25ms).
func (Inventory) Check(productID string, quantity int) CheckResult {
	elapsed := time.Duration(5+rand.N(20)) * time.Millisecond
	time.Sleep(elapsed)
	return CheckResult{
		InStock:  true,
		LowStock: rand.N(100) < 10,
		Elapsed:  elapsed,
	}
}
