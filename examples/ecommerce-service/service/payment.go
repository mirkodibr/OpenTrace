package service

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
)

// ErrPaymentDeclined is the simulated payment failure (~5% of charges).
var ErrPaymentDeclined = errors.New("payment declined by provider")

// Payments simulates a payment provider client.
type Payments struct{}

// ChargeResult describes a simulated charge attempt.
type ChargeResult struct {
	PaymentID string
	Elapsed   time.Duration
}

// Charge simulates a payment authorisation (20-80ms, 5% declines).
func (Payments) Charge(userID string, amount float64) (ChargeResult, error) {
	elapsed := time.Duration(20+rand.N(60)) * time.Millisecond
	time.Sleep(elapsed)
	if rand.N(100) < 5 {
		return ChargeResult{Elapsed: elapsed},
			fmt.Errorf("charging %s: %w", userID, ErrPaymentDeclined)
	}
	return ChargeResult{
		PaymentID: "pay_" + uuid.NewString()[:8],
		Elapsed:   elapsed,
	}, nil
}
