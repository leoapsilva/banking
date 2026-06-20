package c6

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/upwifi/banking/internal/provider/c6/client"
	webhookdomain "github.com/upwifi/banking/internal/webhook/domain"
)

// WebhookAdapter implements webhook.RegistrarProvider and
// webhook.InboundParser against the C6 Notifications API.
type WebhookAdapter struct {
	client *client.Client
}

func NewWebhookAdapter(c *client.Client) *WebhookAdapter {
	return &WebhookAdapter{client: c}
}

type registerWebhookRequest struct {
	URL     string `json:"url"`
	Service string `json:"service"`
}

func (a *WebhookAdapter) Register(ctx context.Context, req webhookdomain.RegisterWebhookRequest) error {
	body := registerWebhookRequest{URL: req.URL, Service: string(req.Service)}
	return a.client.Do(ctx, http.MethodPost, "/", body, nil)
}

func (a *WebhookAdapter) Delete(ctx context.Context, service webhookdomain.Service) error {
	return a.client.Do(ctx, http.MethodDelete, "/?service="+string(service), nil, nil)
}

// c6InboundNotification mirrors the WebhookNotification schema: external_id,
// date_time, client_id, partner_id, service, status, information (a string
// containing serialized JSON whose shape depends on "service").
type c6InboundNotification struct {
	ExternalID  string `json:"external_id"`
	DateTime    string `json:"date_time"`
	ClientID    string `json:"client_id"`
	PartnerID   string `json:"partner_id"`
	Service     string `json:"service"`
	Status      string `json:"status"`
	Information string `json:"information"`
}

// c6CheckoutInformation mirrors the subset of the checkout "information"
// JSON we need: the checkout id and, when a card was saved, its token.
type c6CheckoutInformation struct {
	ID      string `json:"id"`
	Payment struct {
		Card struct {
			CardInfo struct {
				Token string `json:"token"`
			} `json:"card_info"`
		} `json:"card"`
	} `json:"payment"`
}

func (a *WebhookAdapter) ParseInbound(ctx context.Context, rawBody []byte) (webhookdomain.InboundEvent, error) {
	var n c6InboundNotification
	if err := json.Unmarshal(rawBody, &n); err != nil {
		return webhookdomain.InboundEvent{}, fmt.Errorf("c6/webhook: decode notification: %w", err)
	}

	event := webhookdomain.InboundEvent{
		BaaS:           "c6",
		ExternalID:     n.ExternalID,
		ClientID:       n.ClientID,
		PartnerID:      n.PartnerID,
		Service:        webhookdomain.Service(n.Service),
		Status:         n.Status,
		RawInformation: n.Information,
		RawPayload:     rawBody,
	}

	// C6 sends RFC3339 timestamps with nanosecond precision (e.g.
	// "2025-11-27T20:12:12.089629237Z"). This is what makes redelivery
	// idempotency possible: the (baas, external_id, status, event_date_time)
	// unique constraint in webhook_events only dedupes correctly if every
	// inbound event actually carries a concrete, identical timestamp on
	// retransmission — a zero/NULL value here would defeat it, since SQL
	// treats every NULL as distinct.
	if parsed, err := time.Parse(time.RFC3339Nano, n.DateTime); err == nil {
		event.EventDateTime = parsed
	}

	if event.Service == webhookdomain.ServiceCheckout && n.Information != "" {
		var info c6CheckoutInformation
		if err := json.Unmarshal([]byte(n.Information), &info); err == nil {
			event.ProviderCheckoutID = info.ID
			if info.Payment.Card.CardInfo.Token != "" {
				token := info.Payment.Card.CardInfo.Token
				event.SavedCardToken = &token
			}
		}
	}

	return event, nil
}
