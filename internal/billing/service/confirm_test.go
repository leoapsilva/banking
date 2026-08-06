package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	checkoutfeature "github.com/upwifi/banking/internal/checkout"
	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
	checkoutrepo "github.com/upwifi/banking/internal/checkout/repository"
)

// fakeCheckoutStore implements billing.CheckoutStore for tests that never
// reach the concrete *repository.Repository (which needs a real DB).
type fakeCheckoutStore struct {
	getByProviderID func(ctx context.Context, baas checkoutdomain.BaaS, providerCheckoutID string) (checkoutrepo.Checkout, error)
}

func (f *fakeCheckoutStore) GetByID(context.Context, uuid.UUID) (checkoutrepo.Checkout, error) {
	return checkoutrepo.Checkout{}, nil
}

func (f *fakeCheckoutStore) GetByProviderID(ctx context.Context, baas checkoutdomain.BaaS, providerCheckoutID string) (checkoutrepo.Checkout, error) {
	return f.getByProviderID(ctx, baas, providerCheckoutID)
}

func (f *fakeCheckoutStore) UpdateStatus(context.Context, checkoutdomain.BaaS, string, checkoutdomain.Status) error {
	return nil
}

func (f *fakeCheckoutStore) UpdatePaymentDetails(context.Context, uuid.UUID, checkoutrepo.PaymentDetails) error {
	return nil
}

func (f *fakeCheckoutStore) SaveCardToken(context.Context, checkoutrepo.CardToken) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func (f *fakeCheckoutStore) GetCardTokenValue(context.Context, uuid.UUID) (string, error) {
	return "", nil
}

// fakePaymentGateway implements billing.PaymentGateway for the same reason.
type fakePaymentGateway struct {
	checkPayment func(ctx context.Context, baas checkoutdomain.BaaS, req checkoutdomain.CheckPaymentRequest) (checkoutdomain.CheckoutDetails, error)
}

func (f *fakePaymentGateway) CreateCheckout(context.Context, checkoutdomain.CreateCheckoutRequest) (checkoutdomain.CheckoutResult, error) {
	return checkoutdomain.CheckoutResult{}, nil
}

func (f *fakePaymentGateway) Authorize(context.Context, checkoutdomain.AuthorizeRequest) (checkoutdomain.AuthorizeResult, error) {
	return checkoutdomain.AuthorizeResult{}, nil
}

func (f *fakePaymentGateway) CancelCheckout(context.Context, checkoutdomain.BaaS, string) error {
	return nil
}

func (f *fakePaymentGateway) CheckPayment(ctx context.Context, baas checkoutdomain.BaaS, req checkoutdomain.CheckPaymentRequest) (checkoutdomain.CheckoutDetails, error) {
	return f.checkPayment(ctx, baas, req)
}

func TestConfirmInfinitePayPayment_MissingIdentifiersRejected(t *testing.T) {
	svc := New(nil, &fakeCheckoutStore{}, &fakePaymentGateway{})

	_, err := svc.ConfirmInfinitePayPayment(context.Background(), ConfirmInfinitePayPaymentRequest{
		OrderNSU: "ORDER-1", // missing Slug and TransactionNSU
	})
	if err == nil {
		t.Fatal("expected an error when slug/transaction_nsu are missing")
	}
}

func TestConfirmInfinitePayPayment_CheckoutNotFound(t *testing.T) {
	store := &fakeCheckoutStore{
		getByProviderID: func(context.Context, checkoutdomain.BaaS, string) (checkoutrepo.Checkout, error) {
			return checkoutrepo.Checkout{}, errors.New("no rows")
		},
	}
	svc := New(nil, store, &fakePaymentGateway{})

	_, err := svc.ConfirmInfinitePayPayment(context.Background(), ConfirmInfinitePayPaymentRequest{
		OrderNSU: "UNKNOWN", Slug: "s", TransactionNSU: "t",
	})
	if !errors.Is(err, ErrCheckoutNotFound) {
		t.Errorf("err = %v, want ErrCheckoutNotFound", err)
	}
}

// A checkout already PAID (activated by the webhook, another redirect, or
// the reconciliation job) must short-circuit before calling the provider —
// confirming twice is a no-op, not a duplicate activation attempt.
func TestConfirmInfinitePayPayment_AlreadyPaidIsIdempotent(t *testing.T) {
	providerCalled := false
	store := &fakeCheckoutStore{
		getByProviderID: func(context.Context, checkoutdomain.BaaS, string) (checkoutrepo.Checkout, error) {
			return checkoutrepo.Checkout{Status: checkoutdomain.StatusPaid}, nil
		},
	}
	gateway := &fakePaymentGateway{
		checkPayment: func(context.Context, checkoutdomain.BaaS, checkoutdomain.CheckPaymentRequest) (checkoutdomain.CheckoutDetails, error) {
			providerCalled = true
			return checkoutdomain.CheckoutDetails{}, nil
		},
	}
	svc := New(nil, store, gateway)

	result, err := svc.ConfirmInfinitePayPayment(context.Background(), ConfirmInfinitePayPaymentRequest{
		OrderNSU: "ORDER-1", Slug: "s", TransactionNSU: "t",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != ConfirmationAlreadyPaid {
		t.Errorf("result = %q, want %q", result, ConfirmationAlreadyPaid)
	}
	if providerCalled {
		t.Error("payment_check should not be called for an already-paid checkout")
	}
}

func TestConfirmInfinitePayPayment_NotPaidYet(t *testing.T) {
	store := &fakeCheckoutStore{
		getByProviderID: func(context.Context, checkoutdomain.BaaS, string) (checkoutrepo.Checkout, error) {
			return checkoutrepo.Checkout{Status: checkoutdomain.StatusCreated}, nil
		},
	}
	gateway := &fakePaymentGateway{
		checkPayment: func(context.Context, checkoutdomain.BaaS, checkoutdomain.CheckPaymentRequest) (checkoutdomain.CheckoutDetails, error) {
			return checkoutdomain.CheckoutDetails{Status: checkoutdomain.StatusCreated}, nil
		},
	}
	svc := New(nil, store, gateway)

	result, err := svc.ConfirmInfinitePayPayment(context.Background(), ConfirmInfinitePayPaymentRequest{
		OrderNSU: "ORDER-1", Slug: "s", TransactionNSU: "t",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != ConfirmationNotPaidYet {
		t.Errorf("result = %q, want %q", result, ConfirmationNotPaidYet)
	}
}

// ErrNotSupported cannot happen for InfinitePay in practice, but a wiring
// mistake (calling this for a BaaS that doesn't support CheckPayment) must
// fail soft — "not confirmed yet" — rather than surface as a hard error.
func TestConfirmInfinitePayPayment_ProviderNotSupportedFailsSoft(t *testing.T) {
	store := &fakeCheckoutStore{
		getByProviderID: func(context.Context, checkoutdomain.BaaS, string) (checkoutrepo.Checkout, error) {
			return checkoutrepo.Checkout{Status: checkoutdomain.StatusCreated}, nil
		},
	}
	gateway := &fakePaymentGateway{
		checkPayment: func(context.Context, checkoutdomain.BaaS, checkoutdomain.CheckPaymentRequest) (checkoutdomain.CheckoutDetails, error) {
			return checkoutdomain.CheckoutDetails{}, checkoutfeature.ErrNotSupported
		},
	}
	svc := New(nil, store, gateway)

	result, err := svc.ConfirmInfinitePayPayment(context.Background(), ConfirmInfinitePayPaymentRequest{
		OrderNSU: "ORDER-1", Slug: "s", TransactionNSU: "t",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != ConfirmationNotPaidYet {
		t.Errorf("result = %q, want %q", result, ConfirmationNotPaidYet)
	}
}
