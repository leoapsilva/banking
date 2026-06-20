// Package domain holds the BaaS-agnostic bank slip (boleto) model. Every
// provider adapter translates to/from this shape; nothing here may
// reference a specific BaaS.
package domain

import "time"

// BaaS identifies which banking-as-a-service backs a request.
type BaaS string

const (
	BaaSC6 BaaS = "c6"
)

// Status is the unified bank slip lifecycle status.
type Status string

const (
	StatusCreated   Status = "CREATED"
	StatusPaid      Status = "PAID"
	StatusCancelled Status = "CANCELLED"
)

// IsTerminal reports whether the status cannot transition further.
func (s Status) IsTerminal() bool {
	switch s {
	case StatusPaid, StatusCancelled:
		return true
	default:
		return false
	}
}

// Money represents an amount in the smallest currency unit (cents) to avoid
// floating point rounding issues.
type Money struct {
	AmountCents int64
	Currency    string // ISO 4217, e.g. "BRL"
}

// Address is the payer's billing address. Unlike checkout, the C6 boleto
// API requires every one of these fields to issue a slip.
type Address struct {
	Street     string
	Number     string
	Complement string
	City       string
	State      string
	ZipCode    string
}

// Payer is the person/entity responsible for paying the bank slip.
type Payer struct {
	Name    string
	TaxID   string
	Email   string
	Address Address
}

// AmountType describes whether a discount/fine/interest figure is a flat
// amount or a percentage of the slip's principal. Names are deliberately
// neutral so a future non-C6 provider can map to its own vocabulary in its
// adapter (C6 itself uses the single-letter codes "V"/"P").
type AmountType string

const (
	AmountTypeFixed      AmountType = "FIXED"
	AmountTypePercentage AmountType = "PERCENTAGE"
)

// DiscountTier configures one early-payment discount bracket: a payer who
// settles the slip at least DeadlineDays before the due date gets the
// configured discount. C6 supports up to three tiers (first/second/third),
// ordered with strictly decreasing DeadlineDays.
type DiscountTier struct {
	Type AmountType
	// Value is in cents when Type is Fixed, or whole-percentage units
	// (e.g. 10 == 10%) when Type is Percentage.
	Value        int64
	DeadlineDays int
}

// Discount configures early-payment discounts. Tiers must be supplied in
// descending DeadlineDays order (the first tier has the longest lead time);
// C6 rejects ties or an ascending ordering.
type Discount struct {
	Tiers []DiscountTier
}

// Fine configures a one-time late-payment penalty charged once the
// DeadlineDays grace period after the due date has elapsed.
type Fine struct {
	Type AmountType
	// Value is in cents when Type is Fixed, or whole-percentage units
	// (e.g. 2 == 2%) when Type is Percentage.
	Value        int64
	DeadlineDays int
}

// Interest configures daily late-payment interest accrued once the
// DeadlineDays grace period after the due date has elapsed.
type Interest struct {
	Type AmountType
	// Value is in cents/day when Type is Fixed, or whole-percentage-per-month
	// units when Type is Percentage (C6 divides the monthly rate by 30 to
	// derive the daily accrual).
	Value        int64
	DeadlineDays int
}

// CreateBankSlipRequest is the unified payload for issuing a bank slip.
// Discount/Interest/Fine are all optional: a slip issued with none of them
// set still carries a hard payment deadline (DueDate) but accrues no
// late-payment charges.
type CreateBankSlipRequest struct {
	BaaS                BaaS
	ExternalReferenceID string
	Amount              Money
	DueDate             time.Time
	Instructions        []string
	Discount            *Discount
	Interest            *Interest
	Fine                *Fine
	Payer               Payer
}

// BankSlipResult is what providers return after issuing a bank slip.
type BankSlipResult struct {
	ProviderBankSlipID string
	OurNumber          string
	BarCode            string
	DigitableLine      string
	Amount             Money
	DueDate            time.Time
	Status             Status
}

// PaymentInfo records a settlement against the bank slip, populated by the
// provider once the slip has been paid.
type PaymentInfo struct {
	Date   time.Time
	Amount Money
}

// BankSlipDetails is the full state of a bank slip as returned by a GET.
type BankSlipDetails struct {
	ProviderBankSlipID  string
	ExternalReferenceID string
	Status              Status
	Amount              Money
	DueDate             time.Time
	Instructions        []string
	Discount            *Discount
	Interest            *Interest
	Fine                *Fine
	Payer               Payer
	OurNumber           string
	BarCode             string
	DigitableLine       string
	Payments            []PaymentInfo
}

// UpdateBankSlipRequest carries the alterable subset of a bank slip's
// fields. A nil field means "leave unchanged" — C6 does not support
// clearing a previously set discount/interest/fine via this endpoint, only
// replacing it.
type UpdateBankSlipRequest struct {
	Amount   *Money
	DueDate  *time.Time
	Discount *Discount
	Interest *Interest
	Fine     *Fine
}
