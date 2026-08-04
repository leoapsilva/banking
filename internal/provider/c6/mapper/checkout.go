// Package mapper holds pure functions translating between our unified
// checkout domain model and C6's DTOs. Kept side-effect free and isolated
// so they can be table-driven tested without any HTTP/DB dependency.
package mapper

import (
	"encoding/json"
	"fmt"

	"github.com/upwifi/banking/internal/checkout/domain"
	"github.com/upwifi/banking/internal/provider/c6/dto"
)

// ToC6InterestType translates our neutral interest vocabulary to C6's.
// CUSTOMER_ABSORBED -> BY_ISSUER ("Parcelado Emissor": cardholder pays interest).
// MERCHANT_ABSORBED -> BY_SELLER ("Parcelado Loja": merchant absorbs the cost).
func ToC6InterestType(t domain.InterestType) (string, error) {
	switch t {
	case domain.InterestCustomerAbsorbed:
		return "BY_ISSUER", nil
	case domain.InterestMerchantAbsorbed:
		return "BY_SELLER", nil
	default:
		return "", fmt.Errorf("c6/mapper: unknown interest type %q", t)
	}
}

// FromC6Status translates C6 checkout statuses to our unified Status enum.
func FromC6Status(s string) domain.Status {
	switch s {
	case "CREATED":
		return domain.StatusCreated
	case "IN PROGRESS", "AUTHORIZED, CONFIRMATION PENDING", "CONFIRMATION REQUESTED", "CANCELLATION REQUESTED":
		return domain.StatusInProgress
	case "PAID":
		return domain.StatusPaid
	case "DECLINED":
		return domain.StatusDeclined
	case "EXPIRED":
		return domain.StatusExpired
	case "CANCELLED":
		return domain.StatusCancelled
	default:
		return domain.StatusError
	}
}

func toAddress(a *domain.Address) *dto.Address {
	if a == nil {
		return nil
	}
	return &dto.Address{
		Street: a.Street, Number: json.Number(a.Number), Complement: a.Complement,
		City: a.City, State: a.State, ZipCode: a.ZipCode,
	}
}

func toPayer(p *domain.Payer) *dto.Payer {
	if p == nil {
		return nil
	}
	return &dto.Payer{
		Name: p.Name, TaxID: p.TaxID, Email: p.Email, PhoneNumber: p.PhoneNumber,
		Address: toAddress(p.Address),
	}
}

func toAmount(m domain.Money) float64 {
	return float64(m.AmountCents) / 100
}

func toCardInfoValue(c *domain.CardInfo) string {
	if c == nil {
		return ""
	}
	if c.Token != nil {
		return *c.Token
	}
	if c.CardHash != nil {
		return *c.CardHash
	}
	return ""
}

// ToC6CardPayment translates a unified card payment configuration.
func ToC6CardPayment(c *domain.CardPayment) (*dto.CardPayment, error) {
	if c == nil {
		return nil, nil
	}
	card := &dto.CardPayment{
		Type:              string(c.Type),
		Installments:      c.Installments,
		FixedInstallments: true,
		Recurrent:         c.Recurrent,
		SaveCard:          c.SaveCard,
		Capture:           c.Capture,
		Authenticate:      string(c.Authentication),
		CardInfo:          toCardInfoValue(c.CardInfo),
	}
	if c.InterestType != nil {
		it, err := ToC6InterestType(*c.InterestType)
		if err != nil {
			return nil, err
		}
		card.InterestType = it
	} else {
		// C6's own example requests always populate interest_type, even
		// for a single-installment (non-financed) purchase, so we default
		// to BY_SELLER (merchant-absorbed — there's no interest to begin
		// with when installments == 1) rather than omitting the field.
		card.InterestType = "BY_SELLER"
	}
	return card, nil
}

// c6AutoPix is sent alongside payment.card on every checkout/authorize
// request, matching C6's own example payloads which always populate both
// payment.card and payment.pix (the hosted checkout page offers Pix as an
// alternative payment method to the payer regardless of which method the
// caller is configuring programmatically).
var c6AutoPix = &dto.Pix{Key: "AUTO"}

// ToC6CreateCheckoutRequest builds the C6 POST /v1/checkouts/ body.
func ToC6CreateCheckoutRequest(req domain.CreateCheckoutRequest) (dto.CreateCheckoutRequest, error) {
	card, err := ToC6CardPayment(req.Card)
	if err != nil {
		return dto.CreateCheckoutRequest{}, err
	}
	return dto.CreateCheckoutRequest{
		Amount:              toAmount(req.Amount),
		Description:         req.Description,
		ExternalReferenceID: req.ExternalReferenceID,
		Payer:               toPayer(req.Payer),
		Payment:             dto.Payment{Card: card, Pix: c6AutoPix},
		RedirectURL:         req.RedirectURL,
	}, nil
}

// ToC6AuthorizeRequest builds the C6 POST /v1/checkouts/authorize body.
func ToC6AuthorizeRequest(req domain.AuthorizeRequest) (dto.AuthorizeRequest, error) {
	card, err := ToC6CardPayment(req.Card)
	if err != nil {
		return dto.AuthorizeRequest{}, err
	}
	return dto.AuthorizeRequest{
		Amount:              toAmount(req.Amount),
		Description:         req.Description,
		ExternalReferenceID: req.ExternalReferenceID,
		Payer:               toPayer(req.Payer),
		Payment:             dto.Payment{Card: card, Pix: c6AutoPix},
	}, nil
}

// FromC6CreateCheckoutResponse builds our unified result from C6's create response.
func FromC6CreateCheckoutResponse(resp dto.CreateCheckoutResponse, initialStatus domain.Status) domain.CheckoutResult {
	return domain.CheckoutResult{
		ProviderCheckoutID: resp.ID,
		CheckoutURL:        resp.URL,
		Status:             initialStatus,
	}
}

// FromC6GetCheckoutResponse builds our unified checkout details.
func FromC6GetCheckoutResponse(resp dto.GetCheckoutResponse) domain.CheckoutDetails {
	details := domain.CheckoutDetails{
		ProviderCheckoutID: resp.ID,
		Status:             FromC6Status(resp.Status),
		Amount:             domain.Money{AmountCents: int64(resp.Amount * 100), Currency: "BRL"},
	}
	if resp.Payment.Card != nil && resp.Payment.Card.CardInfo != nil && resp.Payment.Card.CardInfo.Token != "" {
		token := resp.Payment.Card.CardInfo.Token
		details.SavedCardToken = &token
	}
	return details
}

// FromC6AuthorizeResponse builds our unified authorize result.
func FromC6AuthorizeResponse(resp dto.AuthorizeResponse) domain.AuthorizeResult {
	result := domain.AuthorizeResult{
		ProviderCheckoutID: resp.ID,
		Status:             FromC6Status(resp.Status),
	}
	if resp.Payment.Card != nil {
		result.ReturnCode = resp.Payment.Card.ReturnCode
		result.Message = resp.Payment.Card.Message
		if resp.Payment.Card.CardInfo != nil && resp.Payment.Card.CardInfo.Token != "" {
			token := resp.Payment.Card.CardInfo.Token
			result.SavedCardToken = &token
		}
	}
	return result
}
