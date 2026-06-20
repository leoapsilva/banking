package dto

import "encoding/json"

// BoletoAddress mirrors C6's "address" schema for bank slips. Number is
// numeric in C6's schema (it marshals as a bare literal via json.Number),
// matching the checkout DTO's same convention — see
// internal/provider/c6/dto/checkout.go.
type BoletoAddress struct {
	Street     string      `json:"street"`
	Number     json.Number `json:"number"`
	Complement string      `json:"complement,omitempty"`
	City       string      `json:"city"`
	State      string      `json:"state"`
	ZipCode    string      `json:"zip_code"`
}

// BoletoPayer mirrors C6's "payer" schema for bank slips. Unlike checkout's
// Payer, name/tax_id/address are required by C6 to issue a slip.
type BoletoPayer struct {
	Name    string        `json:"name"`
	TaxID   string        `json:"tax_id"`
	Email   string        `json:"email,omitempty"`
	Address BoletoAddress `json:"address"`
}

// DiscountDetails mirrors C6's "discount_details" schema: one early-payment
// discount bracket.
type DiscountDetails struct {
	Value    float64 `json:"value"`
	DeadLine int     `json:"dead_line"`
}

// Discount mirrors C6's "discount" schema. DiscountType is "V" (fixed
// value) or "P" (percentage); up to three brackets may be configured, with
// strictly decreasing DeadLine across First/Second/Third.
type Discount struct {
	DiscountType string           `json:"discount_type,omitempty"`
	First        *DiscountDetails `json:"first,omitempty"`
	Second       *DiscountDetails `json:"second,omitempty"`
	Third        *DiscountDetails `json:"third,omitempty"`
}

// Fine mirrors C6's "fine" schema: a one-time late-payment penalty.
type Fine struct {
	Type     string  `json:"type,omitempty"`
	Value    float64 `json:"value,omitempty"`
	DeadLine int     `json:"dead_line,omitempty"`
}

// Interest mirrors C6's "interest" schema: daily late-payment interest.
type Interest struct {
	Type     string  `json:"type,omitempty"`
	Value    float64 `json:"value,omitempty"`
	DeadLine int     `json:"dead_line,omitempty"`
}

// CreateBankSlipRequest mirrors C6's "bank_slip_create_request" schema
// (POST /v1/bank_slips/).
type CreateBankSlipRequest struct {
	ExternalReferenceID string      `json:"external_reference_id"`
	Amount              float64     `json:"amount"`
	DueDate             string      `json:"due_date"` // YYYY-MM-DD
	Instructions        []string    `json:"instructions,omitempty"`
	Discount            *Discount   `json:"discount,omitempty"`
	Interest            *Interest   `json:"interest,omitempty"`
	Fine                *Fine       `json:"fine,omitempty"`
	Payer               BoletoPayer `json:"payer"`
}

// CreateBankSlipResponse mirrors C6's "bank_slip_create_response" schema.
type CreateBankSlipResponse struct {
	Amount        float64 `json:"amount"`
	DueDate       string  `json:"due_date"`
	OriginatorID  string  `json:"originator_id"`
	OurNumber     string  `json:"our_number"`
	BillingScheme string  `json:"billing_scheme"`
	BillingType   string  `json:"billing_type"`
	ID            string  `json:"id"`
	BarCode       string  `json:"bar_code"`
	DigitableLine string  `json:"digitable_line"`
}

// BoletoPayment mirrors C6's "payment" schema: one settlement record
// against the slip, only ever populated by C6 in responses. Named
// BoletoPayment (not Payment) to avoid colliding with checkout's own
// Payment type in this shared dto package.
type BoletoPayment struct {
	Date   string  `json:"date,omitempty"`
	Amount float64 `json:"amount,omitempty"`
}

// BankSlipResponse mirrors C6's "bank_slip_response" schema, returned by
// both the create-confirmation read path and GET /{id}.
type BankSlipResponse struct {
	ID                  string          `json:"id"`
	OriginatorID        string          `json:"originator_id,omitempty"`
	ExternalReferenceID string          `json:"external_reference_id,omitempty"`
	Amount              float64         `json:"amount"`
	Status              string          `json:"status,omitempty"`
	EmissionDate        string          `json:"emission_date,omitempty"`
	DueDate             string          `json:"due_date"`
	Instructions        []string        `json:"instructions,omitempty"`
	Payments            []BoletoPayment `json:"payments,omitempty"`
	InternalID          string          `json:"internal_id,omitempty"`
	BillingScheme       string          `json:"billing_scheme,omitempty"`
	BillingType         string          `json:"billing_type,omitempty"`
	DigitableLine       string          `json:"digitable_line,omitempty"`
	BarCode             string          `json:"bar_code,omitempty"`
	Discount            *Discount       `json:"discount,omitempty"`
	Interest            *Interest       `json:"interest,omitempty"`
	Fine                *Fine           `json:"fine,omitempty"`
	OurNumber           string          `json:"our_number,omitempty"`
	Payer               BoletoPayer     `json:"payer"`
	Base64PDFFile       string          `json:"base64_pdf_file,omitempty"`
}

// AlterBankSlipRequest mirrors C6's "bank_slip_alter_request" schema
// (PUT /v1/bank_slips/{id}). minProperties: 1 — at least one field must be
// set, which the mapper/service enforce before building this struct.
type AlterBankSlipRequest struct {
	Amount   *float64  `json:"amount,omitempty"`
	DueDate  *string   `json:"due_date,omitempty"`
	Discount *Discount `json:"discount,omitempty"`
	Interest *Interest `json:"interest,omitempty"`
	Fine     *Fine     `json:"fine,omitempty"`
}
