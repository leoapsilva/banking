// Package dto mirrors the JSON shapes C6's APIs accept/return, kept
// separate from our unified domain model so adapter mappers stay the only
// place that knows both vocabularies.
package dto

import "encoding/json"

type Address struct {
	Street string `json:"street"`
	// Number is numeric in C6's schema (unlike our own unified
	// domain.Address.Number, which is a string to stay flexible for
	// addressing schemes that mix letters in). json.Number marshals as a
	// bare numeric literal, matching what C6 expects.
	Number     json.Number `json:"number,omitempty"`
	Complement string      `json:"complement,omitempty"`
	City       string      `json:"city"`
	State      string      `json:"state"`
	ZipCode    string      `json:"zip_code"`
}

type Payer struct {
	Name        string   `json:"name,omitempty"`
	TaxID       string   `json:"tax_id,omitempty"`
	Email       string   `json:"email,omitempty"`
	PhoneNumber string   `json:"phone_number,omitempty"`
	Address     *Address `json:"address,omitempty"`
}

type CardPayment struct {
	Type         string `json:"type"`
	Installments int    `json:"installments"`
	InterestType string `json:"interest_type,omitempty"`
	// FixedInstallments mirrors C6's writeOnly "fixed_installments" field
	// (default true in their schema, and always present in their own
	// example requests) — whether the payer can choose the installment
	// count or it's fixed at the value above. We always send true since
	// our unified domain model doesn't yet expose payer-selectable
	// installments as a concept.
	FixedInstallments bool   `json:"fixed_installments"`
	Recurrent         bool   `json:"recurrent,omitempty"`
	SaveCard          bool   `json:"save_card,omitempty"`
	Capture           bool   `json:"capture"`
	Authenticate      string `json:"authenticate,omitempty"`
	CardInfo          string `json:"card_info,omitempty"` // oneOf card_hash | token, sent as a bare string
}

type Pix struct {
	Key string `json:"key,omitempty"`
}

type Payment struct {
	Card *CardPayment `json:"card,omitempty"`
	Pix  *Pix         `json:"pix,omitempty"`
}

type CreateCheckoutRequest struct {
	Amount              float64 `json:"amount"`
	Description         string  `json:"description,omitempty"`
	ExternalReferenceID string  `json:"external_reference_id,omitempty"`
	Payer               *Payer  `json:"payer,omitempty"`
	Payment             Payment `json:"payment"`
	RedirectURL         string  `json:"redirect_url,omitempty"`
}

type CreateCheckoutResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type CardInfoResponse struct {
	Number     string `json:"number,omitempty"`
	Brand      string `json:"brand,omitempty"`
	HolderName string `json:"holder_name,omitempty"`
	Token      string `json:"token,omitempty"`
}

type CardPaymentResponse struct {
	Type              string            `json:"type,omitempty"`
	Installments      int               `json:"installments,omitempty"`
	AuthorizationCode string            `json:"authorization_code,omitempty"`
	Message           string            `json:"message,omitempty"`
	ReturnCode        string            `json:"return_code,omitempty"`
	CardInfo          *CardInfoResponse `json:"card_info,omitempty"`
}

type PaymentResponse struct {
	Card *CardPaymentResponse `json:"card,omitempty"`
	Pix  *Pix                 `json:"pix,omitempty"`
}

type GetCheckoutResponse struct {
	ID      string          `json:"id"`
	Amount  float64         `json:"amount"`
	Status  string          `json:"status"`
	Payment PaymentResponse `json:"payment"`
}

type AuthorizeRequest struct {
	Amount              float64 `json:"amount"`
	Description         string  `json:"description,omitempty"`
	ExternalReferenceID string  `json:"external_reference_id"`
	Payer               *Payer  `json:"payer,omitempty"`
	Payment             Payment `json:"payment"`
}

type AuthorizeResponse struct {
	ID      string          `json:"id"`
	Status  string          `json:"status"`
	Payment PaymentResponse `json:"payment"`
}
