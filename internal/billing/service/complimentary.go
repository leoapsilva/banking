package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/upwifi/banking/internal/billing/repository"
)

// GrantComplimentaryRequest records a decision to stop charging a
// subscription — a waived annual fee, a lifetime exemption, or goodwill from
// support after a problem.
//
// Reason and GrantedBy are mandatory: a free subscription with no recorded
// justification is indistinguishable from a billing bug when someone reviews
// the numbers months later.
type GrantComplimentaryRequest struct {
	SubscriptionID uuid.UUID
	Until          *time.Time // nil together with Forever=true
	Forever        bool
	Reason         string
	GrantedBy      string
}

// GrantComplimentary exempts a subscription from billing.
//
// This is deliberately not a coupon. A 100% discount would mean a zero-value
// charge — rejected by providers, and it would bury a commercial decision
// inside pricing arithmetic. Exemption is its own concept, with its own audit
// trail.
func (s *Service) GrantComplimentary(ctx context.Context, req GrantComplimentaryRequest) error {
	if req.Reason == "" || req.GrantedBy == "" {
		return fmt.Errorf("billing: complimentary grant requires reason and granted_by")
	}
	if !req.Forever && req.Until == nil {
		return fmt.Errorf("billing: complimentary grant requires either until or forever")
	}
	if req.Forever && req.Until != nil {
		return fmt.Errorf("billing: complimentary grant cannot be both forever and bounded")
	}
	if req.Until != nil && req.Until.Before(time.Now()) {
		return fmt.Errorf("billing: complimentary grant cannot end in the past")
	}

	if err := s.repo.GrantComplimentary(ctx, req.SubscriptionID, repository.ComplimentaryGrant{
		Until:     req.Until,
		Forever:   req.Forever,
		Reason:    req.Reason,
		GrantedBy: req.GrantedBy,
	}); err != nil {
		return err
	}

	// Free access is worth an explicit audit line, not just a row update.
	slog.Info("billing: complimentary access granted",
		"subscription_id", req.SubscriptionID,
		"forever", req.Forever,
		"until", req.Until,
		"reason", req.Reason,
		"granted_by", req.GrantedBy,
	)
	return nil
}

// ExtendNextCharge pushes a subscription's next charge date forward, which is
// how support waives a single upcoming cycle without making the customer
// permanently free.
func (s *Service) ExtendNextCharge(ctx context.Context, subID uuid.UUID, newDate time.Time, grantedBy string) error {
	if newDate.Before(time.Now()) {
		return fmt.Errorf("billing: cannot move the next charge into the past")
	}
	if err := s.repo.AdvanceNextChargeDate(ctx, subID, newDate); err != nil {
		return err
	}
	slog.Info("billing: next charge postponed",
		"subscription_id", subID, "new_date", newDate, "granted_by", grantedBy)
	return nil
}
