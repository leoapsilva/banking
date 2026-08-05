package mapper

import (
	"testing"

	"github.com/upwifi/banking/internal/checkout/domain"
	"github.com/upwifi/banking/internal/provider/infinitepay/dto"
)

func TestToCreateLinkRequest_MinimalRequest(t *testing.T) {
	req := domain.CreateCheckoutRequest{
		Amount:      domain.Money{AmountCents: 15000, Currency: "BRL"},
		Description: "Plano Anual",
	}
	got := ToCreateLinkRequest("myhandle", "https://banking.example.com/webhooks/infinitepay/secret", req)

	if got.Handle != "myhandle" {
		t.Errorf("Handle = %q, want %q", got.Handle, "myhandle")
	}
	if len(got.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(got.Items))
	}
	if got.Items[0].Price != 15000 {
		t.Errorf("Items[0].Price = %d, want 15000", got.Items[0].Price)
	}
	if got.Items[0].Quantity != 1 {
		t.Errorf("Items[0].Quantity = %d, want 1", got.Items[0].Quantity)
	}
	if got.Items[0].Description != "Plano Anual" {
		t.Errorf("Items[0].Description = %q, want %q", got.Items[0].Description, "Plano Anual")
	}
	if got.Customer != nil {
		t.Errorf("Customer should be nil for request without Payer")
	}
	if got.Address != nil {
		t.Errorf("Address should be nil for request without Payer")
	}
}

func TestToCreateLinkRequest_EmptyDescriptionDefaultsToPagamento(t *testing.T) {
	req := domain.CreateCheckoutRequest{
		Amount: domain.Money{AmountCents: 5000, Currency: "BRL"},
	}
	got := ToCreateLinkRequest("h", "", req)
	if got.Items[0].Description != "Pagamento" {
		t.Errorf("empty description should default to %q, got %q", "Pagamento", got.Items[0].Description)
	}
}

func TestToCreateLinkRequest_WithPayer(t *testing.T) {
	req := domain.CreateCheckoutRequest{
		Amount:      domain.Money{AmountCents: 20000, Currency: "BRL"},
		Description: "Assinatura",
		Payer: &domain.Payer{
			Name:        "Maria Souza",
			Email:       "maria@email.com",
			PhoneNumber: "11999998888",
		},
		RedirectURL: "https://seusite.com/obrigado",
	}
	got := ToCreateLinkRequest("handle", "https://wh.example.com/infinitepay/s", req)

	if got.Customer == nil {
		t.Fatal("Customer should not be nil when Payer is set")
	}
	if got.Customer.Name != "Maria Souza" {
		t.Errorf("Customer.Name = %q, want %q", got.Customer.Name, "Maria Souza")
	}
	if got.Customer.Email != "maria@email.com" {
		t.Errorf("Customer.Email = %q, want %q", got.Customer.Email, "maria@email.com")
	}
	if got.Customer.PhoneNumber != "11999998888" {
		t.Errorf("Customer.PhoneNumber = %q, want %q", got.Customer.PhoneNumber, "11999998888")
	}
	if got.Address != nil {
		t.Errorf("Address should be nil when Payer has no Address")
	}
	if got.RedirectURL != "https://seusite.com/obrigado" {
		t.Errorf("RedirectURL = %q, want %q", got.RedirectURL, "https://seusite.com/obrigado")
	}
}

func TestToCreateLinkRequest_WithPayerAndAddress(t *testing.T) {
	req := domain.CreateCheckoutRequest{
		Amount:      domain.Money{AmountCents: 10000, Currency: "BRL"},
		Description: "Teste",
		Payer: &domain.Payer{
			Name: "João",
			Address: &domain.Address{
				ZipCode:      "01310100",
				Street:       "Av. Paulista",
				Neighborhood: "Bela Vista",
				Number:       "1000",
				Complement:   "Apto 42",
			},
		},
	}
	got := ToCreateLinkRequest("h", "", req)

	if got.Address == nil {
		t.Fatal("Address should not be nil when Payer has Address")
	}
	if got.Address.Cep != "01310100" {
		t.Errorf("Address.Cep = %q, want %q", got.Address.Cep, "01310100")
	}
	if got.Address.Street != "Av. Paulista" {
		t.Errorf("Address.Street = %q, want %q", got.Address.Street, "Av. Paulista")
	}
	if got.Address.Neighborhood != "Bela Vista" {
		t.Errorf("Address.Neighborhood = %q, want %q", got.Address.Neighborhood, "Bela Vista")
	}
	if got.Address.Number != "1000" {
		t.Errorf("Address.Number = %q, want %q", got.Address.Number, "1000")
	}
	if got.Address.Complement != "Apto 42" {
		t.Errorf("Address.Complement = %q, want %q", got.Address.Complement, "Apto 42")
	}
}

func TestToCreateLinkRequest_WebhookURLAndOrderNSU(t *testing.T) {
	req := domain.CreateCheckoutRequest{
		Amount:              domain.Money{AmountCents: 1000, Currency: "BRL"},
		Description:         "x",
		ExternalReferenceID: "ORDER-001",
	}
	webhookURL := "https://banking.example.com/webhooks/infinitepay/mysecret"
	got := ToCreateLinkRequest("h", webhookURL, req)

	if got.WebhookURL != webhookURL {
		t.Errorf("WebhookURL = %q, want %q", got.WebhookURL, webhookURL)
	}
	if got.OrderNSU != "ORDER-001" {
		t.Errorf("OrderNSU = %q, want %q", got.OrderNSU, "ORDER-001")
	}
}

// ProviderCheckoutID must come from the order_nsu we generated, not
// resp.Slug — verified against a real sandbox call where POST /links
// returned only `url`, no `slug` field at all, despite the documented
// response shape. Using resp.Slug made ProviderCheckoutID always empty,
// which broke webhook correlation and collided with the (baas,
// provider_checkout_id) unique constraint on every checkout after the
// first.
func TestFromCreateLinkResponse(t *testing.T) {
	resp := dto.CreateLinkResponse{
		URL: "https://checkout.infinitepay.io/cores-app-br?lenc=abc123",
	}
	got := FromCreateLinkResponse(resp, "ORDER-001")

	if got.ProviderCheckoutID != "ORDER-001" {
		t.Errorf("ProviderCheckoutID = %q, want %q", got.ProviderCheckoutID, "ORDER-001")
	}
	if got.CheckoutURL != "https://checkout.infinitepay.io/cores-app-br?lenc=abc123" {
		t.Errorf("CheckoutURL = %q, want %q", got.CheckoutURL, "https://checkout.infinitepay.io/cores-app-br?lenc=abc123")
	}
	if got.Status != domain.StatusCreated {
		t.Errorf("Status = %q, want %q", got.Status, domain.StatusCreated)
	}
}

// Even if InfinitePay starts returning a slug in the future, order_nsu must
// still win: the webhook only ever echoes back order_nsu, so using anything
// else as ProviderCheckoutID would make correlation impossible regardless
// of what the creation response contains.
func TestFromCreateLinkResponseIgnoresSlugEvenIfPresent(t *testing.T) {
	resp := dto.CreateLinkResponse{
		Slug: "some-slug-infinitepay-might-add-later",
		URL:  "https://checkout.infinitepay.io/cores-app-br?lenc=abc123",
	}
	got := FromCreateLinkResponse(resp, "ORDER-001")

	if got.ProviderCheckoutID != "ORDER-001" {
		t.Errorf("ProviderCheckoutID = %q, want %q (order_nsu must win over slug)", got.ProviderCheckoutID, "ORDER-001")
	}
}
