package mapper

import (
	"testing"
	"time"

	"github.com/upwifi/banking/internal/pix/domain"
	"github.com/upwifi/banking/internal/provider/c6/dto"
)

func TestToC6Amount(t *testing.T) {
	cases := []struct {
		name  string
		cents int64
		want  string
	}{
		{"whole reais", 10000, "100.00"},
		{"with cents", 12345, "123.45"},
		{"single cent", 1, "0.01"},
		{"zero", 0, "0.00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToC6Amount(tc.cents); got != tc.want {
				t.Errorf("ToC6Amount(%d) = %q, want %q", tc.cents, got, tc.want)
			}
		})
	}
}

func TestFromC6Amount(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    int64
		wantErr bool
	}{
		{"whole reais", "100.00", 10000, false},
		{"with cents", "123.45", 12345, false},
		{"empty string", "", 0, false},
		{"invalid", "abc", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FromC6Amount(tc.in)
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
				t.Errorf("FromC6Amount(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestAmountRoundTrip(t *testing.T) {
	cents := int64(37050)
	s := ToC6Amount(cents)
	got, err := FromC6Amount(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != cents {
		t.Errorf("round trip got %d, want %d", got, cents)
	}
}

func TestFromC6PixStatus(t *testing.T) {
	cases := []struct {
		in   string
		want domain.Status
	}{
		{"ATIVA", domain.StatusActive},
		{"CONCLUIDA", domain.StatusCompleted},
		{"REMOVIDA_PELO_USUARIO_RECEBEDOR", domain.StatusRemovedByReceiver},
		{"REMOVIDA_PELO_PSP", domain.StatusRemovedByPSP},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := FromC6PixStatus(tc.in); got != tc.want {
				t.Errorf("FromC6PixStatus(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestToC6CobDevedor(t *testing.T) {
	t.Run("nil debtor returns nil without error", func(t *testing.T) {
		got, err := ToC6CobDevedor(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("cpf debtor", func(t *testing.T) {
		got, err := ToC6CobDevedor(&domain.Debtor{
			DocumentType: domain.DebtorCPF, DocumentID: "12345678909", Name: "Francisco da Silva",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.CPF != "12345678909" || got.CNPJ != "" || got.Nome != "Francisco da Silva" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("cnpj debtor", func(t *testing.T) {
		got, err := ToC6CobDevedor(&domain.Debtor{
			DocumentType: domain.DebtorCNPJ, DocumentID: "12345678000195", Name: "Empresa de Serviços SA",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.CNPJ != "12345678000195" || got.CPF != "" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("unknown document type errors", func(t *testing.T) {
		_, err := ToC6CobDevedor(&domain.Debtor{DocumentType: "BOGUS", DocumentID: "x", Name: "x"})
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})
}

func TestToC6CobVDevedor_RequiresAddress(t *testing.T) {
	got, err := ToC6CobVDevedor(domain.Debtor{
		DocumentType: domain.DebtorCPF, DocumentID: "12345678909", Name: "Francisco da Silva",
		Street: "Alameda Souza, Numero 80, Bairro Braz", City: "Recife", State: "PE", ZipCode: "70011750",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Logradouro != "Alameda Souza, Numero 80, Bairro Braz" || got.Cidade != "Recife" || got.UF != "PE" || got.CEP != "70011750" {
		t.Errorf("got %+v", got)
	}
}

func TestToC6CreateCobRequest(t *testing.T) {
	req := domain.CreateImmediateChargeRequest{
		Amount:            domain.Money{AmountCents: 3700},
		PixKey:            "minha-chave-pix",
		ExpirationSeconds: 3600,
		Debtor: &domain.Debtor{
			DocumentType: domain.DebtorCNPJ, DocumentID: "12345678000195", Name: "Empresa de Serviços SA",
		},
		PayerRequest: "Serviço realizado.",
	}
	got, err := ToC6CreateCobRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Valor.Original != "37.00" {
		t.Errorf("got valor.original %q, want %q", got.Valor.Original, "37.00")
	}
	if got.Calendario.Expiracao != 3600 {
		t.Errorf("got expiracao %d, want 3600", got.Calendario.Expiracao)
	}
	if got.Chave != "minha-chave-pix" {
		t.Errorf("got chave %q", got.Chave)
	}
	if got.Devedor == nil || got.Devedor.CNPJ != "12345678000195" {
		t.Errorf("got devedor %+v", got.Devedor)
	}
}

func TestFromC6CobResponse(t *testing.T) {
	resp := dto.CobResponse{
		Calendario: dto.CobCalendario{Criacao: "2025-09-09T20:15:00Z", Expiracao: 3600},
		TxID:       "7978c0c97ea847e78e8849634473c1f1",
		Revisao:    0,
		Location:   "pix.example.com/qr/9d36b84fc70b478fb95c12729b90ca25",
		Status:     "ATIVA",
		Devedor:    &dto.CobDevedor{CNPJ: "12345678000195", Nome: "Empresa de Serviços SA"},
		Valor:      dto.CobValor{Original: "37.00"},
		Chave:      "minha-chave-pix",
	}
	got, err := FromC6CobResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Kind != domain.ChargeImmediate {
		t.Errorf("got kind %q, want IMMEDIATE", got.Kind)
	}
	if got.Status != domain.StatusActive {
		t.Errorf("got status %q, want ACTIVE", got.Status)
	}
	if got.Amount.AmountCents != 3700 {
		t.Errorf("got amount %d, want 3700", got.Amount.AmountCents)
	}
	if got.Debtor == nil || got.Debtor.DocumentType != domain.DebtorCNPJ {
		t.Errorf("got debtor %+v", got.Debtor)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("expected CreatedAt to be parsed")
	}
}

func TestToC6CreateCobVRequest(t *testing.T) {
	dueDate := time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)
	req := domain.CreateChargeWithDueDateRequest{
		TxID:                  "fb2761260e554ad593c7226beb5cb650",
		Amount:                domain.Money{AmountCents: 25100},
		PixKey:                "minha-chave-pix",
		DueDate:               dueDate,
		DaysValidAfterDueDate: 30,
		Debtor: domain.Debtor{
			DocumentType: domain.DebtorCPF, DocumentID: "12345678909", Name: "Francisco da Silva",
			Street: "Alameda Souza, Numero 80, Bairro Braz", City: "Recife", State: "PE", ZipCode: "70011750",
		},
	}
	got, err := ToC6CreateCobVRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Calendario.DataDeVencimento != "2025-06-30" {
		t.Errorf("got dataDeVencimento %q, want 2025-06-30", got.Calendario.DataDeVencimento)
	}
	if got.Calendario.ValidadeAposVencimento != 30 {
		t.Errorf("got validadeAposVencimento %d, want 30", got.Calendario.ValidadeAposVencimento)
	}
	if got.Valor.Original != "251.00" {
		t.Errorf("got valor.original %q, want 251.00", got.Valor.Original)
	}
	if got.Devedor.CPF != "12345678909" {
		t.Errorf("got devedor.cpf %q", got.Devedor.CPF)
	}
}

func TestFromC6CobVResponse(t *testing.T) {
	resp := dto.CobVResponse{
		Calendario: dto.CobVCalendario{
			Criacao: "2025-09-09T20:15:00Z", DataDeVencimento: "2025-12-31", ValidadeAposVencimento: 30,
		},
		TxID:    "7978c0c97ea847e78e8849634473c1f1",
		Revisao: 0,
		Status:  "ATIVA",
		Devedor: dto.CobVDevedor{
			CPF: "12345678909", Nome: "Francisco da Silva",
			Logradouro: "Alameda Souza, Numero 80, Bairro Braz", Cidade: "Recife", UF: "PE", CEP: "70011750",
		},
		Valor: dto.CobValor{Original: "123.45"},
		Chave: "5f84a4c5-c5cb-4599-9f13-7eb4d419dacc",
	}
	got, err := FromC6CobVResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Kind != domain.ChargeWithDueDate {
		t.Errorf("got kind %q, want WITH_DUE_DATE", got.Kind)
	}
	if got.DueDate == nil || got.DueDate.Format("2006-01-02") != "2025-12-31" {
		t.Errorf("got due date %v", got.DueDate)
	}
	if got.Amount.AmountCents != 12345 {
		t.Errorf("got amount %d, want 12345", got.Amount.AmountCents)
	}
	if got.Debtor == nil || got.Debtor.Street != "Alameda Souza, Numero 80, Bairro Braz" {
		t.Errorf("got debtor %+v", got.Debtor)
	}
}

func TestFromC6CobsConsultadas(t *testing.T) {
	resp := dto.CobsConsultadasResponse{
		Cobs: []dto.CobResponse{
			{TxID: "tx1", Status: "ATIVA", Valor: dto.CobValor{Original: "10.00"}},
			{TxID: "tx2", Status: "CONCLUIDA", Valor: dto.CobValor{Original: "20.00"}},
		},
	}
	got, err := FromC6CobsConsultadas(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Charges) != 2 {
		t.Fatalf("got %d charges, want 2", len(got.Charges))
	}
	if got.Charges[0].TxID != "tx1" || got.Charges[1].TxID != "tx2" {
		t.Errorf("got %+v", got.Charges)
	}
}

func TestToC6WebhookSolicitado(t *testing.T) {
	got := ToC6WebhookSolicitado(domain.RegisterWebhookRequest{URL: "https://minhaempresa.com.br/webhook/pix"})
	if got.WebhookURL != "https://minhaempresa.com.br/webhook/pix" {
		t.Errorf("got %q", got.WebhookURL)
	}
}
