// Package domain holds the BaaS-agnostic checkout model. Every provider
// adapter translates to/from this shape; nothing here may reference a
// specific BaaS.
package domain

import "time"

// BaaS identifies which banking-as-a-service backs a request.
type BaaS string

const (
	BaaSC6 BaaS = "c6"
)

// CardType is the unified card transaction type.
type CardType string

const (
	CardTypeDebit  CardType = "DEBIT"
	CardTypeCredit CardType = "CREDIT"
)

// InterestType describes who bears the interest on an installment purchase.
// Names are deliberately neutral so a future non-C6 provider can map to its
// own vocabulary in its adapter.
type InterestType string

const (
	InterestMerchantAbsorbed InterestType = "MERCHANT_ABSORBED" // loja assume o custo (C6: BY_SELLER)
	InterestCustomerAbsorbed InterestType = "CUSTOMER_ABSORBED" // cliente assume o custo (C6: BY_ISSUER)
)

// AuthenticationRequirement mirrors whether 3DS-style authentication should
// be attempted by the issuer.
type AuthenticationRequirement string

const (
	AuthRequired    AuthenticationRequirement = "REQUIRED"
	AuthOptional    AuthenticationRequirement = "OPTIONAL"
	AuthNotRequired AuthenticationRequirement = "NOT_REQUIRED"
)

// Status is the unified checkout lifecycle status.
type Status string

const (
	StatusCreated    Status = "CREATED"
	StatusInProgress Status = "IN_PROGRESS"
	StatusPaid       Status = "PAID"
	StatusDeclined   Status = "DECLINED"
	StatusExpired    Status = "EXPIRED"
	StatusCancelled  Status = "CANCELLED"
	StatusError      Status = "ERROR"
)

// IsTerminal reports whether the status cannot transition further.
func (s Status) IsTerminal() bool {
	switch s {
	case StatusPaid, StatusDeclined, StatusExpired, StatusCancelled, StatusError:
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

// Address is the payer's billing address.
type Address struct {
	Street     string
	Number     string
	Complement string
	City       string
	State      string
	ZipCode    string
}

// Payer is the person/entity paying the checkout.
type Payer struct {
	Name        string
	TaxID       string
	Email       string
	PhoneNumber string
	Address     *Address
}

// CardInfo identifies the card used for payment: either a previously saved
// token (recurring/subsequent charges) or a tokenized card hash captured at
// the front-end during the original checkout (transparent checkout flow).
type CardInfo struct {
	Token    *string
	CardHash *string
}

// CardPayment configures a card transaction.
type CardPayment struct {
	Type           CardType
	Installments   int           // 1-12
	InterestType   *InterestType // required if Installments > 1
	Recurrent      bool
	SaveCard       bool
	Capture        bool // manual capture not yet supported by C6; always true today
	Authentication AuthenticationRequirement
	CardInfo       *CardInfo
}

// CreateCheckoutRequest is the unified payload for creating a checkout.
type CreateCheckoutRequest struct {
	BaaS                BaaS
	ExternalReferenceID string
	Amount              Money
	Description         string
	Payer               *Payer
	Card                *CardPayment
	RedirectURL         string
	ExpirationDateTime  *time.Time
}

// CheckoutResult is what providers return after creating a checkout.
type CheckoutResult struct {
	ProviderCheckoutID string
	CheckoutURL        string
	Status             Status
}

// AuthorizeRequest charges a previously tokenized card directly, without a
// hosted checkout page. Used for subsequent recurring billing cycles.
type AuthorizeRequest struct {
	BaaS                BaaS
	ExternalReferenceID string
	Amount              Money
	Description         string
	Payer               *Payer
	Card                *CardPayment // CardInfo.Token must be set
}

// AuthorizeResult is the outcome of a direct authorization.
type AuthorizeResult struct {
	ProviderCheckoutID string
	Status             Status
	ReturnCode         string
	Message            string
	SavedCardToken     *string // populated when SaveCard was requested and approved
}

// CheckoutDetails is the full state of a checkout as returned by a GET.
type CheckoutDetails struct {
	ProviderCheckoutID string
	Status             Status
	Amount             Money
	SavedCardToken     *string
}
