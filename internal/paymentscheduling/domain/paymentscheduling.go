// Package domain models DDA consultation and payment scheduling per C6's
// "Agendamento de Pagamentos" API (docs/c6/swagger/agendamento-de-pagamentos.yaml)
// — item 8 of the conformity roteiro. This API does not execute payments
// itself: it submits boletos/PIX for pre-processing (decode), batches them
// into a "payment group", and that group must be submitted for approval by
// an authorized web banking user before anything is actually paid.
package domain

import "time"

// BaaS identifies which banking-as-a-service backs a request.
type BaaS string

const (
	BaaSC6 BaaS = "c6"
)

// ProductType is what kind of payment a content string decodes to.
type ProductType string

const (
	ProductBoleto ProductType = "BOLETO"
	ProductPix    ProductType = "PIX"
)

// PaymentStatus mirrors C6's "status" enum for a decoded payment item.
type PaymentStatus string

const (
	StatusReadData            PaymentStatus = "READ_DATA"            // item accepted, not yet decoded
	StatusError               PaymentStatus = "ERROR"                // payment could not be completed
	StatusDecodeError         PaymentStatus = "DECODE_ERROR"         // content could not be decoded — check input
	StatusProcessed           PaymentStatus = "PROCESSED"            // paid successfully
	StatusScheduled           PaymentStatus = "SCHEDULED"            // approved and scheduled
	StatusProcessing          PaymentStatus = "PROCESSING"           // payment in flight
	StatusSchedulingCancelled PaymentStatus = "SCHEDULING_CANCELLED" // scheduling was cancelled
)

// Money represents an amount in the smallest currency unit (cents) to avoid
// floating point rounding issues. C6's API uses a decimal number, not
// cents, so the adapter mapper handles the conversion.
type Money struct {
	AmountCents int64
}

// DDABond is a pending bill registered against the authenticated PJ's CPNJ
// in the DDA (Débito Direto Autorizado) registry — item 8.1. C6's "bonds"
// schema; read-only, returned by GET /query.
type DDABond struct {
	Amount          Money
	BankCode        string
	BankName        string
	BeneficiaryName string
	Content         string // the bar code to submit for decode if scheduling this bond
	DueDate         time.Time
	Overdue         bool
	PayerName       string
}

// PaymentInput is what the caller submits for decode (POST /decode) — item
// 8.2. Content is either a boleto bar code, a PIX BR Code, or a raw PIX
// key; C6 itself determines which from the value's shape. Boletos found via
// GetDDA can be submitted directly using their Content field, but content
// from any other source works too — the roteiro explicitly notes items
// don't need to come from a DDA query.
type PaymentInput struct {
	Content         string // required
	Amount          Money  // required
	BankCode        string
	BankName        string
	BeneficiaryName string
	Description     string // free text shown on the approval screen, max 100 chars, not validated by C6
	PayerName       string
	TransactionDate *time.Time // when the payment should execute; C6 defaults to "today" if omitted
}

// PaymentItem is a payment as tracked inside a payment group — the shape
// returned by GET /{group_id}/items and as elements submitted to /decode.
// Fields marked read-only in the swagger (DueDate, ErrorMessage, GroupID,
// ID, Overdue, ProductType, Status) are only populated on results, never
// required on input.
type PaymentItem struct {
	ID              string
	GroupID         string
	Content         string
	Amount          Money
	BankCode        string
	BankName        string
	BeneficiaryName string
	Description     string
	PayerName       string
	DueDate         *time.Time
	TransactionDate *time.Time
	Overdue         *bool
	ProductType     ProductType
	Status          PaymentStatus
	ErrorMessage    string
}

// SubmitForDecodeRequest batches payments for initial processing — item
// 8.2. C6 validates each item individually (bar code integrity / already
// paid for boletos; key integrity / DICT lookup for PIX) and returns a
// group_id to track them.
type SubmitForDecodeRequest struct {
	BaaS  BaaS
	Items []PaymentInput
}

// SubmitForDecodeResult is what C6 returns after accepting a batch.
type SubmitForDecodeResult struct {
	GroupID string
}

// SubmitForApprovalRequest sends a previously decoded group for human
// approval in web banking — item 8.6. Once submitted, the group can no
// longer be edited via API.
type SubmitForApprovalRequest struct {
	BaaS         BaaS
	GroupID      string
	UploaderName string
}
