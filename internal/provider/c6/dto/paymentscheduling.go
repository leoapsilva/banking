// Package dto (this file) mirrors C6's "Agendamento de Pagamentos" JSON
// shapes (docs/c6/swagger/agendamento-de-pagamentos.yaml). Only
// internal/provider/c6/mapper is allowed to know both this vocabulary and
// the unified paymentscheduling/domain one.
package dto

type PaymentInput struct {
	Content         string  `json:"content"`
	Amount          float64 `json:"amount"`
	BankCode        string  `json:"bank_code,omitempty"`
	BankName        string  `json:"bank_name,omitempty"`
	BeneficiaryName string  `json:"beneficiary_name,omitempty"`
	Description     string  `json:"description,omitempty"`
	PayerName       string  `json:"payer_name,omitempty"`
	TransactionDate string  `json:"transaction_date,omitempty"`
}

// DecodeRequest is the body for POST /decode.
type DecodeRequest struct {
	Items []PaymentInput `json:"items"`
}

// DecodeResponse is the response for POST /decode.
type DecodeResponse struct {
	GroupID string `json:"group_id"`
}

// Bond mirrors C6's "bonds" schema — a DDA-registered boleto pending
// payment, returned by GET /query.
type Bond struct {
	Amount          float64 `json:"amount"`
	BankCode        string  `json:"bank_code"`
	BankName        string  `json:"bank_name"`
	BeneficiaryName string  `json:"beneficiary_name"`
	Content         string  `json:"content"`
	DueDate         string  `json:"due_date"`
	Overdue         bool    `json:"overdue"`
	PayerName       string  `json:"payer_name"`
}

// QueryResponse is the response for GET /query.
type QueryResponse struct {
	Items []Bond `json:"items"`
}

// PaymentItem mirrors C6's "payment" schema as returned by
// GET /{group_id}/items — includes the read-only fields (id, group_id,
// status, product_type, due_date, overdue, error_message) absent on input.
type PaymentItem struct {
	ID              string  `json:"id,omitempty"`
	GroupID         string  `json:"group_id,omitempty"`
	Content         string  `json:"content"`
	Amount          float64 `json:"amount"`
	BankCode        string  `json:"bank_code,omitempty"`
	BankName        string  `json:"bank_name,omitempty"`
	BeneficiaryName string  `json:"beneficiary_name,omitempty"`
	Description     string  `json:"description,omitempty"`
	PayerName       string  `json:"payer_name,omitempty"`
	DueDate         string  `json:"due_date,omitempty"`
	TransactionDate string  `json:"transaction_date,omitempty"`
	Overdue         *bool   `json:"overdue,omitempty"`
	ProductType     string  `json:"product_type,omitempty"`
	Status          string  `json:"status,omitempty"`
	ErrorMessage    string  `json:"error_message,omitempty"`
}

// GroupItemsResponse is the response for GET /{group_id}/items.
type GroupItemsResponse struct {
	Items []PaymentItem `json:"items"`
}

// DeleteItemRef is one element of the array body for DELETE
// /{group_id}/items.
type DeleteItemRef struct {
	ID string `json:"id"`
}

// SubmitForApprovalRequest is the body for POST /submit.
type SubmitForApprovalRequest struct {
	GroupID      string `json:"group_id"`
	UploaderName string `json:"uploader_name"`
}
