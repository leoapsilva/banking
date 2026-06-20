package domain

import (
	"time"

	"github.com/google/uuid"
	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
)

// Frequency is how often a subscription bills.
type Frequency string

const (
	FrequencyMonthly Frequency = "MONTHLY"
	FrequencyAnnual  Frequency = "ANNUAL"
)

// Status is the subscription lifecycle.
type Status string

const (
	StatusPendingFirstPayment Status = "PENDING_FIRST_PAYMENT"
	StatusActive              Status = "ACTIVE"
	StatusPastDue             Status = "PAST_DUE"
	StatusCancelled           Status = "CANCELLED"
	StatusCompleted           Status = "COMPLETED"
)

// MaxConsecutiveFailures is how many failed recurring charges in a row
// before a subscription is automatically marked PAST_DUE.
const MaxConsecutiveFailures = 3

// Subscription represents either a monthly recurring charge ("assinatura
// mensal") or an annual installment purchase ("assinatura anual" / 12x with
// interest assumed by the customer). Both share the same aggregate because
// the cancellation and token-management logic is identical; they differ in
// Frequency/InstallmentsTotal and in how charges are triggered.
type Subscription struct {
	ID                  uuid.UUID
	BaaS                checkoutdomain.BaaS
	CustomerTaxID       string
	CustomerName        string
	CustomerEmail       string
	Amount              checkoutdomain.Money
	Frequency           Frequency
	InstallmentsTotal   *int // nil = open-ended monthly recurrence
	InstallmentsDone    int
	InterestType        *checkoutdomain.InterestType
	CardTokenID         *uuid.UUID
	InitialCheckoutID   *uuid.UUID
	Status              Status
	NextChargeDate      *time.Time
	ConsecutiveFailures int
	CancelledAt         *time.Time
	CancelledReason     string
}

// IsRecurringCharge reports whether this subscription needs the cron
// worker to keep billing it (monthly recurrence), as opposed to an annual
// 12x purchase that is fully settled by a single initial checkout.
func (s Subscription) IsRecurringCharge() bool {
	return s.Frequency == FrequencyMonthly
}
