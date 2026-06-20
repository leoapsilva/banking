package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
	checkoutrepo "github.com/upwifi/banking/internal/checkout/repository"
	checkoutservice "github.com/upwifi/banking/internal/checkout/service"
	"github.com/upwifi/banking/internal/subscription/domain"
	"github.com/upwifi/banking/internal/subscription/repository"
	webhookdomain "github.com/upwifi/banking/internal/webhook/domain"
	"github.com/upwifi/banking/pkg/idgen"
)

// Service implements the subscription use cases: creation of monthly
// recurring subscriptions and annual installment purchases, the recurring
// charge cycle driven by the cron worker, cancellation, and reacting to
// checkout webhook events (it implements webhook.CheckoutEventSink).
type Service struct {
	repo         *repository.Repository
	checkoutRepo *checkoutrepo.Repository
	checkoutSvc  *checkoutservice.Service
}

func New(repo *repository.Repository, checkoutRepo *checkoutrepo.Repository, checkoutSvc *checkoutservice.Service) *Service {
	return &Service{repo: repo, checkoutRepo: checkoutRepo, checkoutSvc: checkoutSvc}
}

// CreateMonthlySubscriptionRequest creates an open-ended monthly recurring
// charge. The first charge happens via a hosted checkout (so the payer can
// enter card details); subsequent charges use the saved token.
type CreateMonthlySubscriptionRequest struct {
	BaaS          checkoutdomain.BaaS
	CustomerName  string
	CustomerTaxID string
	CustomerEmail string
	Amount        checkoutdomain.Money
	RedirectURL   string
}

func (s *Service) CreateMonthlySubscription(ctx context.Context, req CreateMonthlySubscriptionRequest) (checkoutdomain.CheckoutResult, uuid.UUID, error) {
	sub := domain.Subscription{
		BaaS:          req.BaaS,
		CustomerTaxID: req.CustomerTaxID,
		CustomerName:  req.CustomerName,
		CustomerEmail: req.CustomerEmail,
		Amount:        req.Amount,
		Frequency:     domain.FrequencyMonthly,
		Status:        domain.StatusPendingFirstPayment,
	}
	subID, err := s.repo.Create(ctx, sub)
	if err != nil {
		return checkoutdomain.CheckoutResult{}, uuid.Nil, fmt.Errorf("subscription: persist: %w", err)
	}

	result, err := s.checkoutSvc.CreateCheckout(ctx, checkoutdomain.CreateCheckoutRequest{
		BaaS:                req.BaaS,
		ExternalReferenceID: idgen.ExternalReferenceID(subID.String(), "first"),
		Amount:              req.Amount,
		Description:         "Assinatura mensal",
		Payer: &checkoutdomain.Payer{
			Name: req.CustomerName, TaxID: req.CustomerTaxID, Email: req.CustomerEmail,
		},
		Card: &checkoutdomain.CardPayment{
			Type:           checkoutdomain.CardTypeCredit,
			Installments:   1,
			Recurrent:      true,
			SaveCard:       true,
			Capture:        true,
			Authentication: checkoutdomain.AuthNotRequired,
		},
		RedirectURL: req.RedirectURL,
	})
	if err != nil {
		return checkoutdomain.CheckoutResult{}, uuid.Nil, fmt.Errorf("subscription: create initial checkout: %w", err)
	}

	checkoutRow, err := s.checkoutRepo.GetByProviderID(ctx, req.BaaS, result.ProviderCheckoutID)
	if err == nil {
		_ = s.repo.SetInitialCheckout(ctx, subID, checkoutRow.ID)
	}

	return result, subID, nil
}

// CreateAnnualInstallmentRequest creates a single checkout with
// installments>1 and interest assumed by the customer (BY_ISSUER on C6).
// Per the C6 model, the card network/issuer splits the charge across the
// cardholder's statement in one authorization, so this subscription needs
// no further recurring charges from our worker; it completes as soon as
// the initial checkout is paid.
type CreateAnnualInstallmentRequest struct {
	BaaS          checkoutdomain.BaaS
	CustomerName  string
	CustomerTaxID string
	CustomerEmail string
	Amount        checkoutdomain.Money
	Installments  int
	RedirectURL   string
}

func (s *Service) CreateAnnualInstallment(ctx context.Context, req CreateAnnualInstallmentRequest) (checkoutdomain.CheckoutResult, uuid.UUID, error) {
	if req.Installments < 2 || req.Installments > 12 {
		return checkoutdomain.CheckoutResult{}, uuid.Nil, fmt.Errorf("subscription: annual installment requires installments between 2 and 12")
	}
	interest := checkoutdomain.InterestCustomerAbsorbed
	installmentsTotal := req.Installments

	sub := domain.Subscription{
		BaaS:              req.BaaS,
		CustomerTaxID:     req.CustomerTaxID,
		CustomerName:      req.CustomerName,
		CustomerEmail:     req.CustomerEmail,
		Amount:            req.Amount,
		Frequency:         domain.FrequencyAnnual,
		InstallmentsTotal: &installmentsTotal,
		InterestType:      &interest,
		Status:            domain.StatusPendingFirstPayment,
	}
	subID, err := s.repo.Create(ctx, sub)
	if err != nil {
		return checkoutdomain.CheckoutResult{}, uuid.Nil, fmt.Errorf("subscription: persist: %w", err)
	}

	result, err := s.checkoutSvc.CreateCheckout(ctx, checkoutdomain.CreateCheckoutRequest{
		BaaS:                req.BaaS,
		ExternalReferenceID: idgen.ExternalReferenceID(subID.String(), "annual"),
		Amount:              req.Amount,
		Description:         "Assinatura anual parcelada",
		Payer: &checkoutdomain.Payer{
			Name: req.CustomerName, TaxID: req.CustomerTaxID, Email: req.CustomerEmail,
		},
		Card: &checkoutdomain.CardPayment{
			Type:           checkoutdomain.CardTypeCredit,
			Installments:   req.Installments,
			InterestType:   &interest,
			SaveCard:       false,
			Capture:        true,
			Authentication: checkoutdomain.AuthNotRequired,
		},
		RedirectURL: req.RedirectURL,
	})
	if err != nil {
		return checkoutdomain.CheckoutResult{}, uuid.Nil, fmt.Errorf("subscription: create initial checkout: %w", err)
	}

	checkoutRow, err := s.checkoutRepo.GetByProviderID(ctx, req.BaaS, result.ProviderCheckoutID)
	if err == nil {
		_ = s.repo.SetInitialCheckout(ctx, subID, checkoutRow.ID)
	}

	return result, subID, nil
}

// HandleCheckoutEvent implements webhook.CheckoutEventSink. It reacts to
// the initial checkout reaching a terminal state: PAID activates (monthly)
// or completes (annual) the subscription; DECLINED/EXPIRED/ERROR cancel it
// since there is no recurring relationship to keep without a first
// successful payment.
func (s *Service) HandleCheckoutEvent(ctx context.Context, event webhookdomain.InboundEvent) error {
	if event.ProviderCheckoutID == "" {
		slog.Warn("subscription: checkout event without provider checkout id, ignoring")
		return nil
	}

	checkoutRow, err := s.checkoutRepo.GetByProviderID(ctx, checkoutdomain.BaaS(event.BaaS), event.ProviderCheckoutID)
	if err != nil {
		// Not every checkout belongs to a subscription (future: one-off
		// checkouts). Not finding it here is not an error condition.
		return nil
	}

	sub, err := s.findSubscriptionByInitialCheckout(ctx, checkoutRow.ID)
	if err != nil || sub == nil {
		return nil
	}

	switch event.Status {
	case string(checkoutdomain.StatusPaid):
		return s.activateFromPaidEvent(ctx, *sub, checkoutRow.ID, event)
	case string(checkoutdomain.StatusDeclined), string(checkoutdomain.StatusExpired), string(checkoutdomain.StatusError):
		return s.repo.Cancel(ctx, sub.ID, "initial checkout "+event.Status)
	}
	return nil
}

func (s *Service) findSubscriptionByInitialCheckout(ctx context.Context, checkoutID uuid.UUID) (*domain.Subscription, error) {
	// Repository only exposes GetByID for subscriptions today; a dedicated
	// lookup by initial_checkout_id would normally live in repository, but
	// for fase 1 volume a direct query here keeps the surface small. If
	// this feature grows, move this into repository.Repository.
	return s.repo.FindByInitialCheckoutID(ctx, checkoutID)
}

func (s *Service) activateFromPaidEvent(ctx context.Context, sub domain.Subscription, checkoutID uuid.UUID, event webhookdomain.InboundEvent) error {
	if sub.Frequency == domain.FrequencyAnnual {
		return s.repo.Complete(ctx, sub.ID)
	}

	if event.SavedCardToken == nil {
		slog.Error("subscription: paid event missing saved card token, cannot activate recurring billing", "subscription_id", sub.ID)
		return fmt.Errorf("subscription: missing saved card token on paid event")
	}

	tokenID, err := s.checkoutRepo.SaveCardToken(ctx, checkoutrepo.CardToken{
		BaaS:             sub.BaaS,
		ProviderToken:    *event.SavedCardToken,
		CustomerTaxID:    sub.CustomerTaxID,
		SourceCheckoutID: checkoutID,
	})
	if err != nil {
		return fmt.Errorf("subscription: save card token: %w", err)
	}

	next := nextMonthlyChargeDate(time.Now())
	return s.repo.ActivateWithToken(ctx, sub.ID, tokenID, &next)
}

func nextMonthlyChargeDate(from time.Time) time.Time {
	return from.AddDate(0, 1, 0)
}

// CancelSubscriptionRequest configures how to handle the current billing
// cycle when a subscription is cancelled.
type CancelSubscriptionRequest struct {
	SubscriptionID     uuid.UUID
	Reason             string
	RefundCurrentCycle bool
}

// CancelSubscription stops future billing immediately, then optionally
// cancels/refunds the current cycle's checkout depending on its state.
func (s *Service) CancelSubscription(ctx context.Context, req CancelSubscriptionRequest) error {
	sub, err := s.repo.GetByID(ctx, req.SubscriptionID)
	if err != nil {
		return fmt.Errorf("subscription: lookup: %w", err)
	}
	if sub.Status == domain.StatusCancelled || sub.Status == domain.StatusCompleted {
		return nil // idempotent: already in a terminal state
	}

	if err := s.repo.Cancel(ctx, sub.ID, req.Reason); err != nil {
		return fmt.Errorf("subscription: cancel: %w", err)
	}

	// Ongoing monthly cycles are billed via direct /authorize calls (no
	// hosted checkout object survives between cycles), so there is nothing
	// async to abort there: setting status=CANCELLED above already stops
	// the cron worker from selecting this subscription again. The only
	// checkout that can still be pending/cancellable is the *initial* one,
	// covering: (a) a monthly subscription cancelled before its first
	// payment completed, and (b) an annual installment purchase, since its
	// initial checkout is the only charge that ever exists for it.
	if sub.Status != domain.StatusPendingFirstPayment && sub.Frequency != domain.FrequencyAnnual {
		return nil
	}
	if sub.InitialCheckoutID == nil {
		return nil
	}

	checkoutRow, err := s.checkoutRepo.GetByID(ctx, *sub.InitialCheckoutID)
	if err != nil {
		return fmt.Errorf("subscription: load initial checkout: %w", err)
	}
	if checkoutRow.Status.IsTerminal() && checkoutRow.Status != checkoutdomain.StatusPaid {
		return nil // already cancelled/declined/expired/error, nothing to do
	}
	if checkoutRow.Status == checkoutdomain.StatusPaid && !req.RefundCurrentCycle {
		return nil // paid: refund is opt-in, not automatic
	}

	return s.checkoutSvc.CancelCheckout(ctx, sub.BaaS, checkoutRow.ProviderCheckoutID)
}

// ChargeDueSubscriptions is the cron job entrypoint: it finds ACTIVE
// monthly subscriptions whose next_charge_date has arrived and charges
// each one via the saved card token, with idempotency per billing cycle.
func (s *Service) ChargeDueSubscriptions(ctx context.Context) error {
	now := time.Now()
	due, err := s.repo.DueForCharge(ctx, now)
	if err != nil {
		return fmt.Errorf("subscription: query due: %w", err)
	}

	for _, sub := range due {
		if err := s.chargeOne(ctx, sub, now); err != nil {
			slog.Error("subscription: charge cycle failed", "subscription_id", sub.ID, "error", err)
		}
	}
	return nil
}

func (s *Service) chargeOne(ctx context.Context, sub domain.Subscription, cycleDate time.Time) error {
	cycleDay := time.Date(cycleDate.Year(), cycleDate.Month(), cycleDate.Day(), 0, 0, 0, 0, time.UTC)
	externalRef := idgen.ExternalReferenceID(sub.ID.String(), cycleDay.Format("2006-01"))

	chargeID, err := s.repo.RecordChargeAttempt(ctx, sub.ID, cycleDay, externalRef)
	if err != nil {
		return fmt.Errorf("record attempt: %w", err)
	}
	if chargeID == uuid.Nil {
		slog.Info("subscription: charge already attempted this cycle, skipping", "subscription_id", sub.ID)
		return nil
	}

	if sub.CardTokenID == nil {
		return fmt.Errorf("subscription has no saved card token")
	}
	providerToken, err := s.checkoutRepo.GetCardTokenValue(ctx, *sub.CardTokenID)
	if err != nil {
		return fmt.Errorf("load card token: %w", err)
	}

	result, err := s.checkoutSvc.Authorize(ctx, checkoutdomain.AuthorizeRequest{
		BaaS:                sub.BaaS,
		ExternalReferenceID: externalRef,
		Amount:              sub.Amount,
		Description:         "Cobranca recorrente mensal",
		Payer: &checkoutdomain.Payer{
			Name: sub.CustomerName, TaxID: sub.CustomerTaxID, Email: sub.CustomerEmail,
		},
		Card: &checkoutdomain.CardPayment{
			Type:           checkoutdomain.CardTypeCredit,
			Installments:   1,
			Recurrent:      true,
			Capture:        true,
			Authentication: checkoutdomain.AuthNotRequired,
			CardInfo:       &checkoutdomain.CardInfo{Token: &providerToken},
		},
	})
	if err != nil {
		_ = s.repo.MarkChargeResult(ctx, chargeID, "FAILED", "", err.Error())
		_ = s.repo.RegisterFailure(ctx, sub.ID)
		return fmt.Errorf("authorize: %w", err)
	}

	if result.Status == checkoutdomain.StatusPaid {
		_ = s.repo.MarkChargeResult(ctx, chargeID, "PAID", result.ReturnCode, result.Message)
		next := nextMonthlyChargeDate(cycleDay)
		return s.repo.AdvanceNextChargeDate(ctx, sub.ID, next)
	}

	_ = s.repo.MarkChargeResult(ctx, chargeID, "FAILED", result.ReturnCode, result.Message)
	return s.repo.RegisterFailure(ctx, sub.ID)
}
