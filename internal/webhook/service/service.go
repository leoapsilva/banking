package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/upwifi/banking/internal/platform/providerregistry"
	webhookfeature "github.com/upwifi/banking/internal/webhook"
	"github.com/upwifi/banking/internal/webhook/domain"
	"github.com/upwifi/banking/internal/webhook/repository"
)

const featureName = "webhook"

// ExpectedOrigin is the set of values we validate incoming notifications
// against, since C6 does not document an HMAC signature for webhooks.
type ExpectedOrigin struct {
	ClientID  string
	PartnerID string
}

type Service struct {
	registry *providerregistry.Registry
	repo     *repository.Repository
	expected ExpectedOrigin

	checkoutSink webhookfeature.CheckoutEventSink
	bankSlipSink webhookfeature.BankSlipEventSink
}

func New(registry *providerregistry.Registry, repo *repository.Repository, expected ExpectedOrigin) *Service {
	return &Service{registry: registry, repo: repo, expected: expected}
}

// SetCheckoutSink registers the handler invoked for CHECKOUT-service
// inbound events. Call once during wiring in main.go.
func (s *Service) SetCheckoutSink(sink webhookfeature.CheckoutEventSink) {
	s.checkoutSink = sink
}

// SetBankSlipSink registers the handler invoked for BANK_SLIP /
// BANK_SLIP_PIX inbound events. Call once during wiring in main.go.
func (s *Service) SetBankSlipSink(sink webhookfeature.BankSlipEventSink) {
	s.bankSlipSink = sink
}

func (s *Service) resolveRegistrar(baas string) (webhookfeature.RegistrarProvider, error) {
	return providerregistry.Get[webhookfeature.RegistrarProvider](s.registry, featureName, baas)
}

func (s *Service) resolveParser(baas string) (webhookfeature.InboundParser, error) {
	return providerregistry.Get[webhookfeature.InboundParser](s.registry, featureName, baas)
}

// RegisterWebhook registers our public callback URL with the BaaS for a
// given service (e.g. CHECKOUT), and persists the registration locally.
func (s *Service) RegisterWebhook(ctx context.Context, baas string, req domain.RegisterWebhookRequest) error {
	registrar, err := s.resolveRegistrar(baas)
	if err != nil {
		return err
	}
	if err := registrar.Register(ctx, req); err != nil {
		return fmt.Errorf("webhook: register with provider: %w", err)
	}
	return s.repo.SaveRegistration(ctx, baas, string(req.Service), req.URL)
}

// ErrUntrustedOrigin is returned when client_id/partner_id on an inbound
// notification don't match what we expect.
var ErrUntrustedOrigin = fmt.Errorf("webhook: untrusted origin")

// ProcessInbound parses, validates, deduplicates, and dispatches a raw
// webhook POST body received from a BaaS. Always returns quickly: heavy
// processing failures are recorded as FAILED_PROCESSING rather than
// propagated, so the BaaS doesn't see a 5xx and retry-storm us — except for
// origin validation failures, which are real rejections.
func (s *Service) ProcessInbound(ctx context.Context, baas string, rawBody []byte) error {
	parser, err := s.resolveParser(baas)
	if err != nil {
		return err
	}

	event, err := parser.ParseInbound(ctx, rawBody)
	if err != nil {
		return fmt.Errorf("webhook: parse inbound: %w", err)
	}

	// Origin validation (client_id / partner_id) only applies to C6: other
	// providers (e.g. InfinitePay) don't include these identifiers in their
	// webhook payloads, so validating them would reject all legitimate events.
	if baas == "c6" {
		if s.expected.ClientID != "" && event.ClientID != s.expected.ClientID {
			return ErrUntrustedOrigin
		}
		if s.expected.PartnerID != "" && event.PartnerID != s.expected.PartnerID {
			return ErrUntrustedOrigin
		}
	}

	result, err := s.repo.InsertEventIfNew(ctx, baas, event.ExternalID, string(event.Service), event.Status, event.EventDateTime, rawBody)
	if err != nil {
		return fmt.Errorf("webhook: persist event: %w", err)
	}
	if result.AlreadyExists {
		slog.Info("webhook: duplicate event ignored", "baas", baas, "external_id", event.ExternalID, "status", event.Status)
		return nil
	}

	var dispatchErr error
	switch event.Service {
	case domain.ServiceCheckout:
		if s.checkoutSink != nil {
			dispatchErr = s.checkoutSink.HandleCheckoutEvent(ctx, event)
		}
	case domain.ServiceBankSlip, domain.ServiceBankSlipPix:
		if s.bankSlipSink != nil {
			dispatchErr = s.bankSlipSink.HandleBankSlipEvent(ctx, event)
		} else {
			slog.Info("webhook: boleto/bolepix event stored, no sink registered", "external_id", event.ExternalID)
		}
	default:
		slog.Warn("webhook: unknown service in inbound event", "service", event.Service)
	}

	if dispatchErr != nil {
		_ = s.repo.MarkFailed(ctx, result.ID)
		slog.Error("webhook: dispatch failed", "error", dispatchErr, "external_id", event.ExternalID)
		return nil
	}

	return s.repo.MarkProcessed(ctx, result.ID)
}
