// Package dto mirrors the JSON shapes InfinitePay's Checkout API
// accepts/returns, kept separate from the unified domain model so adapter
// mappers stay the only place that knows both vocabularies.
package dto

// Item is a line-item in a payment link. InfinitePay requires at least one.
type Item struct {
	Quantity    int    `json:"quantity"`
	Price       int64  `json:"price"` // centavos
	Description string `json:"description"`
}

// Customer optionally pre-fills the hosted checkout page.
type Customer struct {
	Name        string `json:"name,omitempty"`
	Email       string `json:"email,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
}

// Address optionally pre-fills the hosted checkout page.
type Address struct {
	Cep          string `json:"cep,omitempty"`
	Street       string `json:"street,omitempty"`
	Neighborhood string `json:"neighborhood,omitempty"`
	Number       string `json:"number,omitempty"`
	Complement   string `json:"complement,omitempty"`
}

// CreateLinkRequest is the body for POST /links.
type CreateLinkRequest struct {
	Handle      string    `json:"handle"`
	Items       []Item    `json:"items"`
	OrderNSU    string    `json:"order_nsu,omitempty"`
	RedirectURL string    `json:"redirect_url,omitempty"`
	WebhookURL  string    `json:"webhook_url,omitempty"`
	Customer    *Customer `json:"customer,omitempty"`
	Address     *Address  `json:"address,omitempty"`
}

// CreateLinkResponse is the body returned by POST /links.
// The `url` field is the hosted checkout page; `slug` is the stable
// identifier used in payment_check and returned via webhook/redirect.
type CreateLinkResponse struct {
	URL  string `json:"url"`
	Slug string `json:"slug"`
}

// PaymentCheckRequest is the body for POST /payment_check. Slug and
// TransactionNSU only exist after a payment attempt (via webhook or the
// buyer's redirect back) — they cannot be supplied at link-creation time.
type PaymentCheckRequest struct {
	Handle         string `json:"handle"`
	OrderNSU       string `json:"order_nsu"`
	Slug           string `json:"slug"`
	TransactionNSU string `json:"transaction_nsu"`
}

// PaymentCheckResponse is returned by POST /payment_check.
//
// Success is false when the request itself is malformed (e.g. missing
// slug/transaction_nsu) — verified against a real call in production on
// 05/08/2026. It is NOT a signal that the payment was not made; "paid" is
// that signal, and it is only meaningful when Success is true.
type PaymentCheckResponse struct {
	Success       bool    `json:"success"`
	Paid          bool    `json:"paid"`
	Amount        int64   `json:"amount"`
	PaidAmount    int64   `json:"paid_amount"`
	Installments  *int    `json:"installments"`
	CaptureMethod *string `json:"capture_method"`
}

// WebhookPayload is the body InfinitePay POSTs to our webhook_url when a
// payment is approved. InfinitePay only fires this webhook on success, so
// receiving it is sufficient proof of payment (status is implicitly PAID).
type WebhookPayload struct {
	InvoiceSlug    string `json:"invoice_slug"`
	Amount         int64  `json:"amount"`      // centavos — original link amount
	PaidAmount     int64  `json:"paid_amount"` // centavos — may differ (e.g. instalment interest)
	Installments   int    `json:"installments"`
	CaptureMethod  string `json:"capture_method"` // "pix" | "credit_card" | "debit_card"
	TransactionNSU string `json:"transaction_nsu"`
	OrderNSU       string `json:"order_nsu"`
	ReceiptURL     string `json:"receipt_url"`
	Items          []Item `json:"items"`
}
