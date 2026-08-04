package webhook

import (
	"context"

	"github.com/upwifi/banking/internal/webhook/domain"
)

// RegistrarProvider registers/manages outbound webhook subscriptions with a
// BaaS. Implemented once per provider (e.g. internal/provider/c6).
type RegistrarProvider interface {
	Register(ctx context.Context, req domain.RegisterWebhookRequest) error
	Delete(ctx context.Context, service domain.Service) error
}

// InboundParser turns a provider's raw webhook POST body into our unified
// InboundEvent shape. Implemented once per provider, since each BaaS has
// its own notification envelope.
type InboundParser interface {
	ParseInbound(ctx context.Context, rawBody []byte) (domain.InboundEvent, error)
}

// CheckoutEventSink is implemented by the subscription/checkout features
// and registered with the webhook service at wiring time, so this package
// never has to import them directly (avoiding an import cycle, since
// subscription already depends on checkout which would otherwise depend on
// webhook).
type CheckoutEventSink interface {
	HandleCheckoutEvent(ctx context.Context, event domain.InboundEvent) error
}

// BankSlipEventSink is implemented by the boleto feature and registered with
// the webhook service at wiring time (same rationale as CheckoutEventSink).
// It reconciles the notified status onto our local bank slip bookkeeping so
// downstream consumers (e.g. access control / delinquency) can react when a
// boleto is settled (status PAID) or otherwise changes.
type BankSlipEventSink interface {
	HandleBankSlipEvent(ctx context.Context, event domain.InboundEvent) error
}
