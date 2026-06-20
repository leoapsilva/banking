package mapper

import (
	"testing"
	"time"

	"github.com/upwifi/banking/internal/boleto/domain"
	"github.com/upwifi/banking/internal/provider/c6/dto"
)

func TestToC6AmountType(t *testing.T) {
	cases := []struct {
		name    string
		in      domain.AmountType
		want    string
		wantErr bool
	}{
		{"fixed maps to V", domain.AmountTypeFixed, "V", false},
		{"percentage maps to P", domain.AmountTypePercentage, "P", false},
		{"unknown type errors", domain.AmountType("BOGUS"), "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToC6AmountType(tc.in)
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

func TestFromC6BoletoStatus(t *testing.T) {
	cases := []struct {
		in   string
		want domain.Status
	}{
		{"CREATED", domain.StatusCreated},
		{"PAID", domain.StatusPaid},
		{"CANCELLED", domain.StatusCancelled},
		{"SOMETHING_UNEXPECTED", domain.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := FromC6BoletoStatus(tc.in); got != tc.want {
				t.Errorf("FromC6BoletoStatus(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestToC6Fine_FixedAndPercentage(t *testing.T) {
	t.Run("fixed value converts cents to decimal reais", func(t *testing.T) {
		f := &domain.Fine{Type: domain.AmountTypeFixed, Value: 1050, DeadlineDays: 5}
		got, err := ToC6Fine(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Type != "V" || got.Value != 10.5 || got.DeadLine != 5 {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("percentage passes through unscaled", func(t *testing.T) {
		f := &domain.Fine{Type: domain.AmountTypePercentage, Value: 2, DeadlineDays: 0}
		got, err := ToC6Fine(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Type != "P" || got.Value != 2 || got.DeadLine != 0 {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("nil fine returns nil without error", func(t *testing.T) {
		got, err := ToC6Fine(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})
}

func TestFromC6Fine_RoundTrip(t *testing.T) {
	in := &domain.Fine{Type: domain.AmountTypeFixed, Value: 999, DeadlineDays: 3}
	dtoFine, err := ToC6Fine(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := FromC6Fine(dtoFine)
	if out == nil || out.Type != in.Type || out.Value != in.Value || out.DeadlineDays != in.DeadlineDays {
		t.Errorf("round trip mismatch: in=%+v out=%+v", in, out)
	}
}

func TestFromC6Fine_Nil(t *testing.T) {
	if got := FromC6Fine(nil); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
	if got := FromC6Fine(&dto.Fine{}); got != nil {
		t.Errorf("expected nil for empty type, got %+v", got)
	}
}

func TestToC6Interest_FixedAndPercentage(t *testing.T) {
	t.Run("fixed daily value", func(t *testing.T) {
		i := &domain.Interest{Type: domain.AmountTypeFixed, Value: 100, DeadlineDays: 0}
		got, err := ToC6Interest(i)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Type != "V" || got.Value != 1 {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("percentage monthly rate", func(t *testing.T) {
		i := &domain.Interest{Type: domain.AmountTypePercentage, Value: 10, DeadlineDays: 1}
		got, err := ToC6Interest(i)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Type != "P" || got.Value != 10 {
			t.Errorf("got %+v", got)
		}
	})
}

func TestToC6Discount_MultiTier(t *testing.T) {
	d := &domain.Discount{Tiers: []domain.DiscountTier{
		{Type: domain.AmountTypePercentage, Value: 10, DeadlineDays: 10},
		{Type: domain.AmountTypePercentage, Value: 5, DeadlineDays: 5},
	}}
	got, err := ToC6Discount(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.DiscountType != "P" {
		t.Errorf("got discount_type %q, want P", got.DiscountType)
	}
	if got.First == nil || got.First.Value != 10 || got.First.DeadLine != 10 {
		t.Errorf("got First %+v", got.First)
	}
	if got.Second == nil || got.Second.Value != 5 || got.Second.DeadLine != 5 {
		t.Errorf("got Second %+v", got.Second)
	}
	if got.Third != nil {
		t.Errorf("expected Third to be nil, got %+v", got.Third)
	}
}

func TestToC6Discount_TooManyTiers(t *testing.T) {
	d := &domain.Discount{Tiers: []domain.DiscountTier{
		{Type: domain.AmountTypeFixed, Value: 100, DeadlineDays: 10},
		{Type: domain.AmountTypeFixed, Value: 100, DeadlineDays: 7},
		{Type: domain.AmountTypeFixed, Value: 100, DeadlineDays: 5},
		{Type: domain.AmountTypeFixed, Value: 100, DeadlineDays: 3},
	}}
	if _, err := ToC6Discount(d); err == nil {
		t.Fatalf("expected error for 4 tiers, got nil")
	}
}

func TestToC6Discount_NilOrEmpty(t *testing.T) {
	if got, err := ToC6Discount(nil); err != nil || got != nil {
		t.Errorf("expected nil, nil error, got %+v, %v", got, err)
	}
	if got, err := ToC6Discount(&domain.Discount{}); err != nil || got != nil {
		t.Errorf("expected nil for empty tiers, got %+v, %v", got, err)
	}
}

func TestFromC6Discount_RoundTrip(t *testing.T) {
	d := &dto.Discount{
		DiscountType: "V",
		First:        &dto.DiscountDetails{Value: 5, DeadLine: 10},
		Second:       &dto.DiscountDetails{Value: 2.5, DeadLine: 5},
	}
	got := FromC6Discount(d)
	if got == nil || len(got.Tiers) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got.Tiers[0].Type != domain.AmountTypeFixed || got.Tiers[0].Value != 500 || got.Tiers[0].DeadlineDays != 10 {
		t.Errorf("tier 0 mismatch: %+v", got.Tiers[0])
	}
	if got.Tiers[1].Value != 250 || got.Tiers[1].DeadlineDays != 5 {
		t.Errorf("tier 1 mismatch: %+v", got.Tiers[1])
	}
}

func TestToC6CreateBankSlipRequest(t *testing.T) {
	due := time.Date(2025, 6, 11, 0, 0, 0, 0, time.UTC)
	req := domain.CreateBankSlipRequest{
		ExternalReferenceID: "A123456789",
		Amount:              domain.Money{AmountCents: 12345, Currency: "BRL"},
		DueDate:             due,
		Instructions:        []string{"Não receber após o vencimento"},
		Payer: domain.Payer{
			Name:  "José da Silva",
			TaxID: "12345678910",
			Email: "pagador@email.com.br",
			Address: domain.Address{
				Street: "Av. Nove de Julho", Number: "123", City: "Rio de Janeiro",
				State: "RJ", ZipCode: "05093000",
			},
		},
	}
	got, err := ToC6CreateBankSlipRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Amount != 123.45 {
		t.Errorf("got amount %v, want 123.45", got.Amount)
	}
	if got.DueDate != "2025-06-11" {
		t.Errorf("got due_date %q, want 2025-06-11", got.DueDate)
	}
	if got.Payer.Name != "José da Silva" || got.Payer.Address.ZipCode != "05093000" {
		t.Errorf("got payer %+v", got.Payer)
	}
}

func TestFromC6CreateBankSlipResponse(t *testing.T) {
	resp := dto.CreateBankSlipResponse{
		Amount: 123.45, DueDate: "2025-06-11", ID: "01J3NCKY6Q99QC4D7T733D35QD",
		OurNumber: "3048", BarCode: "33695969000000123450000003048720009224128213",
		DigitableLine: "33690.00009",
	}
	got, err := FromC6CreateBankSlipResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Amount.AmountCents != 12345 {
		t.Errorf("got amount cents %d, want 12345", got.Amount.AmountCents)
	}
	if got.Status != domain.StatusCreated {
		t.Errorf("got status %q, want CREATED", got.Status)
	}
	if got.ProviderBankSlipID != "01J3NCKY6Q99QC4D7T733D35QD" {
		t.Errorf("got id %q", got.ProviderBankSlipID)
	}
	if !got.DueDate.Equal(time.Date(2025, 6, 11, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("got due date %v", got.DueDate)
	}
}

func TestFromC6CreateBankSlipResponse_InvalidDueDate(t *testing.T) {
	resp := dto.CreateBankSlipResponse{DueDate: "not-a-date"}
	if _, err := FromC6CreateBankSlipResponse(resp); err == nil {
		t.Fatalf("expected error for invalid due_date")
	}
}

func TestToC6AlterBankSlipRequest_OnlyProvidedFields(t *testing.T) {
	amount := domain.Money{AmountCents: 5000, Currency: "BRL"}
	req := domain.UpdateBankSlipRequest{Amount: &amount}
	got, err := ToC6AlterBankSlipRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Amount == nil || *got.Amount != 50 {
		t.Errorf("got amount %+v", got.Amount)
	}
	if got.DueDate != nil || got.Discount != nil || got.Interest != nil || got.Fine != nil {
		t.Errorf("expected only amount set, got %+v", got)
	}
}

func TestFromC6BankSlipResponse(t *testing.T) {
	resp := dto.BankSlipResponse{
		ID: "01J3NCKY6Q99QC4D7T733D35QD", ExternalReferenceID: "A123456789",
		Amount: 123.45, Status: "PAID", DueDate: "2025-06-11",
		Payer:    dto.BoletoPayer{Name: "José da Silva", TaxID: "12345678910"},
		Payments: []dto.BoletoPayment{{Date: "2023-11-23", Amount: 123.45}},
	}
	got, err := FromC6BankSlipResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != domain.StatusPaid {
		t.Errorf("got status %q, want PAID", got.Status)
	}
	if len(got.Payments) != 1 || got.Payments[0].Amount.AmountCents != 12345 {
		t.Errorf("got payments %+v", got.Payments)
	}
}
