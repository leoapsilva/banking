package domain

import "time"

// Service identifies which product a webhook registration/notification
// belongs to, per C6's vocabulary (shared across providers conceptually).
type Service string

const (
	ServiceBankSlip    Service = "BANK_SLIP"
	ServiceBankSlipPix Service = "BANK_SLIP_PIX"
	ServiceCheckout    Service = "CHECKOUT"
)

// RegisterWebhookRequest registers an outbound notification URL with the BaaS.
type RegisterWebhookRequest struct {
	BaaS    string
	Service Service
	URL     string
}

// InboundEvent is the unified shape of a notification received from a BaaS.
// RawInformation is kept as the provider's raw JSON string/object since its
// schema varies per service (checkout vs. boleto vs. pix). For
// Service == CHECKOUT, the provider's InboundParser also extracts
// ProviderCheckoutID/SavedCardToken so consumers never have to parse the
// provider-specific "information" payload themselves.
type InboundEvent struct {
	BaaS               string
	ExternalID         string
	ClientID           string
	PartnerID          string
	Service            Service
	Status             string
	EventDateTime      time.Time
	RawInformation     string
	RawPayload         []byte
	ProviderCheckoutID string
	SavedCardToken     *string
}
