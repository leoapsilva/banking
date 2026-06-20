// Package domain holds the BaaS-agnostic PIX charge model. Every provider
// adapter translates to/from this shape; nothing here may reference a
// specific BaaS. Field names are deliberately neutral/English even though
// the underlying C6 API (and Brazilian Central Bank PIX spec it implements)
// uses Portuguese vocabulary (txid, devedor, calendario, etc.) — that
// vocabulary is confined to internal/provider/c6/dto and mapper.
package domain

import "time"

// BaaS identifies which banking-as-a-service backs a request.
type BaaS string

const (
	BaaSC6 BaaS = "c6"
)

// ChargeKind distinguishes an immediate charge ("cobrança imediata", C6's
// /cob) from a charge with a due date ("cobrança com vencimento", C6's
// /cobv). The two share most fields but have different calendar semantics.
type ChargeKind string

const (
	ChargeImmediate   ChargeKind = "IMMEDIATE"
	ChargeWithDueDate ChargeKind = "WITH_DUE_DATE"
)

// Status is the unified PIX charge lifecycle status, mirroring C6's
// CobrancaStatus enum (ATIVA/CONCLUIDA/REMOVIDA_PELO_USUARIO_RECEBEDOR/
// REMOVIDA_PELO_PSP) under neutral names.
type Status string

const (
	StatusActive            Status = "ACTIVE"
	StatusCompleted         Status = "COMPLETED"
	StatusRemovedByReceiver Status = "REMOVED_BY_RECEIVER"
	StatusRemovedByPSP      Status = "REMOVED_BY_PSP"
)

// Money represents an amount in the smallest currency unit (cents) to avoid
// floating point rounding issues. PIX amounts are always BRL.
type Money struct {
	AmountCents int64
}

// DebtorDocumentType distinguishes an individual (CPF) from a company
// (CNPJ) debtor, since C6 requires exactly one of the two and validates the
// document format accordingly.
type DebtorDocumentType string

const (
	DebtorCPF  DebtorDocumentType = "CPF"
	DebtorCNPJ DebtorDocumentType = "CNPJ"
)

// Debtor identifies who a charge is addressed to ("devedor" in C6's
// vocabulary) — optional on immediate charges, required on charges with a
// due date.
type Debtor struct {
	DocumentType DebtorDocumentType
	DocumentID   string // digits only; CPF (11) or CNPJ (14)
	Name         string
	// Address fields below are only used/required for charges with a due
	// date (C6's CobVGerada.devedor requires logradouro/cidade/uf/cep).
	Street  string
	City    string
	State   string
	ZipCode string
}

// CreateImmediateChargeRequest creates a "cobrança imediata" (item 7.1/7.2
// of the C6 homologation script): expires a fixed number of seconds after
// creation, optionally identifying a debtor.
type CreateImmediateChargeRequest struct {
	BaaS              BaaS
	TxID              string // optional; if empty, C6 generates one (POST /cob)
	Amount            Money
	PixKey            string // our receiving PIX key ("chave"), required by C6
	ExpirationSeconds int
	Debtor            *Debtor
	PayerRequest      string // "solicitacaoPagador": free text shown to the payer
}

// CreateChargeWithDueDateRequest creates a "cobrança com vencimento" (item
// 7.5): expires on a calendar date, with an additional grace period in days.
type CreateChargeWithDueDateRequest struct {
	BaaS                  BaaS
	TxID                  string // required: C6 has no "PUT without txid" for cobv
	Amount                Money
	PixKey                string
	DueDate               time.Time
	DaysValidAfterDueDate int // "validadeAposVencimento"; C6 default is 30
	Debtor                Debtor
	PayerRequest          string
}

// ChargeResult is what providers return after creating or fetching a charge.
type ChargeResult struct {
	TxID              string
	Kind              ChargeKind
	Status            Status
	Amount            Money
	PixKey            string
	Revision          int
	Location          string // QR code "location" URL, when present
	CreatedAt         time.Time
	ExpirationSeconds int        // immediate charges only
	DueDate           *time.Time // due-date charges only
	Debtor            *Debtor
}

// ListChargesRequest filters a list of charges by creation date range, as
// required by items 7.4/7.6 of the homologation script.
type ListChargesRequest struct {
	BaaS  BaaS
	Start time.Time
	End   time.Time
}

// ListChargesResult is a page of charges matching a ListChargesRequest.
type ListChargesResult struct {
	Charges []ChargeResult
}

// RegisterWebhookRequest configures the C6 PIX product's own webhook (item
// 7.8) — distinct from the generic internal/webhook feature used by
// checkout/boleto, since PIX's webhook is scoped to a single PIX key
// ("chave") rather than a "service" value.
type RegisterWebhookRequest struct {
	BaaS   BaaS
	PixKey string
	URL    string
}
