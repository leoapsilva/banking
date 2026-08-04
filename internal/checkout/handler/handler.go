package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/upwifi/banking/internal/checkout/domain"
	"github.com/upwifi/banking/internal/checkout/service"
)

// Handler exposes the unified checkout HTTP API. Every request carries a
// "baas" field selecting which provider adapter handles it.
type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// Register wires routes onto mux using Go 1.22+ method-aware patterns.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/checkouts", h.create)
	mux.HandleFunc("GET /v1/checkouts/{id}", h.get)
	mux.HandleFunc("PUT /v1/checkouts/{id}/cancel", h.cancel)
}

type addressPayload struct {
	Street       string `json:"street"`
	Number       string `json:"number"`
	Complement   string `json:"complement"`
	Neighborhood string `json:"neighborhood"`
	City         string `json:"city"`
	State        string `json:"state"`
	ZipCode      string `json:"zip_code"`
}

type payerPayload struct {
	Name        string          `json:"name"`
	TaxID       string          `json:"tax_id"`
	Email       string          `json:"email"`
	PhoneNumber string          `json:"phone_number"`
	Address     *addressPayload `json:"address"`
}

type cardInfoPayload struct {
	Token    *string `json:"token"`
	CardHash *string `json:"card_hash"`
}

type cardPayload struct {
	Type           string           `json:"type"`
	Installments   int              `json:"installments"`
	InterestType   *string          `json:"interest_type"`
	Recurrent      bool             `json:"recurrent"`
	SaveCard       bool             `json:"save_card"`
	Capture        *bool            `json:"capture"`
	Authentication string           `json:"authenticate"`
	CardInfo       *cardInfoPayload `json:"card_info"`
}

type createCheckoutPayload struct {
	BaaS                string        `json:"baas"`
	ExternalReferenceID string        `json:"external_reference_id"`
	AmountCents         int64         `json:"amount_cents"`
	Currency            string        `json:"currency"`
	Description         string        `json:"description"`
	Payer               *payerPayload `json:"payer"`
	Card                *cardPayload  `json:"card"`
	RedirectURL         string        `json:"redirect_url"`
}

func (p *cardPayload) toDomain() *domain.CardPayment {
	if p == nil {
		return nil
	}
	capture := true
	if p.Capture != nil {
		capture = *p.Capture
	}
	card := &domain.CardPayment{
		Type:           domain.CardType(p.Type),
		Installments:   p.Installments,
		Recurrent:      p.Recurrent,
		SaveCard:       p.SaveCard,
		Capture:        capture,
		Authentication: domain.AuthenticationRequirement(p.Authentication),
	}
	if p.InterestType != nil {
		it := domain.InterestType(*p.InterestType)
		card.InterestType = &it
	}
	if p.CardInfo != nil {
		card.CardInfo = &domain.CardInfo{Token: p.CardInfo.Token, CardHash: p.CardInfo.CardHash}
	}
	return card
}

func (p *payerPayload) toDomain() *domain.Payer {
	if p == nil {
		return nil
	}
	payer := &domain.Payer{Name: p.Name, TaxID: p.TaxID, Email: p.Email, PhoneNumber: p.PhoneNumber}
	if p.Address != nil {
		payer.Address = &domain.Address{
			Street: p.Address.Street, Number: p.Address.Number, Complement: p.Address.Complement,
			Neighborhood: p.Address.Neighborhood,
			City:         p.Address.City, State: p.Address.State, ZipCode: p.Address.ZipCode,
		}
	}
	return payer
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var payload createCheckoutPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if payload.BaaS == "" {
		writeError(w, http.StatusBadRequest, "baas is required")
		return
	}

	req := domain.CreateCheckoutRequest{
		BaaS:                domain.BaaS(payload.BaaS),
		ExternalReferenceID: payload.ExternalReferenceID,
		Amount:              domain.Money{AmountCents: payload.AmountCents, Currency: payload.Currency},
		Description:         payload.Description,
		Payer:               payload.Payer.toDomain(),
		Card:                payload.Card.toDomain(),
		RedirectURL:         payload.RedirectURL,
	}

	result, err := h.svc.CreateCheckout(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":  result.ProviderCheckoutID,
		"url": result.CheckoutURL,
	})
}

type checkoutDetailsResponse struct {
	ID              string  `json:"id"`
	Status          string  `json:"status"`
	AmountCents     int64   `json:"amount_cents"`
	Currency        string  `json:"currency"`
	SavedCardToken  *string `json:"saved_card_token,omitempty"`
	PaidAmountCents *int64  `json:"paid_amount_cents,omitempty"`
	CaptureMethod   *string `json:"capture_method,omitempty"`
	Installments    *int    `json:"installments,omitempty"`
	ReceiptURL      *string `json:"receipt_url,omitempty"`
	TransactionID   *string `json:"transaction_id,omitempty"`
	PaidAt          *string `json:"paid_at,omitempty"` // RFC3339
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	baas := r.URL.Query().Get("baas")
	if baas == "" {
		writeError(w, http.StatusBadRequest, "baas query parameter is required")
		return
	}

	details, err := h.svc.GetCheckout(r.Context(), domain.BaaS(baas), id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	resp := checkoutDetailsResponse{
		ID:             details.ProviderCheckoutID,
		Status:         string(details.Status),
		AmountCents:    details.Amount.AmountCents,
		Currency:       details.Amount.Currency,
		SavedCardToken: details.SavedCardToken,
		CaptureMethod:  details.CaptureMethod,
		Installments:   details.Installments,
		ReceiptURL:     details.ReceiptURL,
		TransactionID:  details.TransactionID,
	}
	if details.PaidAmount != nil {
		v := details.PaidAmount.AmountCents
		resp.PaidAmountCents = &v
	}
	if details.PaidAt != nil {
		s := details.PaidAt.Format(time.RFC3339)
		resp.PaidAt = &s
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	baas := r.URL.Query().Get("baas")
	if baas == "" {
		writeError(w, http.StatusBadRequest, "baas query parameter is required")
		return
	}

	if err := h.svc.CancelCheckout(r.Context(), domain.BaaS(baas), id); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
