package mapper

import (
	"testing"

	"github.com/upwifi/banking/internal/checkout/domain"
)

func TestToC6InterestType(t *testing.T) {
	cases := []struct {
		name    string
		in      domain.InterestType
		want    string
		wantErr bool
	}{
		{"customer absorbed maps to issuer", domain.InterestCustomerAbsorbed, "BY_ISSUER", false},
		{"merchant absorbed maps to seller", domain.InterestMerchantAbsorbed, "BY_SELLER", false},
		{"unknown type errors", domain.InterestType("BOGUS"), "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToC6InterestType(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFromC6Status(t *testing.T) {
	cases := []struct {
		in   string
		want domain.Status
	}{
		{"CREATED", domain.StatusCreated},
		{"IN PROGRESS", domain.StatusInProgress},
		{"AUTHORIZED, CONFIRMATION PENDING", domain.StatusInProgress},
		{"CONFIRMATION REQUESTED", domain.StatusInProgress},
		{"CANCELLATION REQUESTED", domain.StatusInProgress},
		{"PAID", domain.StatusPaid},
		{"DECLINED", domain.StatusDeclined},
		{"EXPIRED", domain.StatusExpired},
		{"CANCELLED", domain.StatusCancelled},
		{"SOMETHING_UNEXPECTED", domain.StatusError},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := FromC6Status(tc.in); got != tc.want {
				t.Errorf("FromC6Status(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestToC6CardPayment_CardInfoOneOf(t *testing.T) {
	token := "tok_123"
	cardHash := "hash_456"

	t.Run("token takes precedence shape", func(t *testing.T) {
		c := &domain.CardPayment{
			Type: domain.CardTypeCredit, Installments: 1,
			Authentication: domain.AuthNotRequired,
			CardInfo:       &domain.CardInfo{Token: &token},
		}
		got, err := ToC6CardPayment(c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.CardInfo != token {
			t.Errorf("got CardInfo %q, want %q", got.CardInfo, token)
		}
	})

	t.Run("card hash used when token absent", func(t *testing.T) {
		c := &domain.CardPayment{
			Type: domain.CardTypeCredit, Installments: 1,
			Authentication: domain.AuthNotRequired,
			CardInfo:       &domain.CardInfo{CardHash: &cardHash},
		}
		got, err := ToC6CardPayment(c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.CardInfo != cardHash {
			t.Errorf("got CardInfo %q, want %q", got.CardInfo, cardHash)
		}
	})

	t.Run("interest type defaults to BY_SELLER when not set on input", func(t *testing.T) {
		// C6's own example requests always populate interest_type, even at
		// installments=1, so we default rather than omit it.
		c := &domain.CardPayment{
			Type: domain.CardTypeCredit, Installments: 1,
			Authentication: domain.AuthNotRequired,
		}
		got, err := ToC6CardPayment(c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.InterestType != "BY_SELLER" {
			t.Errorf("expected default interest type BY_SELLER, got %q", got.InterestType)
		}
	})

	t.Run("fixed_installments is always sent as true", func(t *testing.T) {
		c := &domain.CardPayment{
			Type: domain.CardTypeCredit, Installments: 1,
			Authentication: domain.AuthNotRequired,
		}
		got, err := ToC6CardPayment(c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.FixedInstallments {
			t.Errorf("expected fixed_installments to be true")
		}
	})

	t.Run("nil card returns nil without error", func(t *testing.T) {
		got, err := ToC6CardPayment(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})
}

func TestToAmount_CentsToDecimal(t *testing.T) {
	got := toAmount(domain.Money{AmountCents: 12345, Currency: "BRL"})
	want := 123.45
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
