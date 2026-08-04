package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/upwifi/banking/internal/webhook/domain"
	"github.com/upwifi/banking/internal/webhook/service"
)

type Handler struct {
	svc                   *service.Service
	c6PathSecret          string
	infinitePayURLToken string
}

// New creates the webhook handler. c6PathSecret is embedded in the C6 inbound
// route for obscurity (C6 documents no HMAC signature). infinitePayURLToken
// serves the same purpose for the InfinitePay inbound route.
func New(svc *service.Service, c6PathSecret, infinitePayURLToken string) *Handler {
	return &Handler{svc: svc, c6PathSecret: c6PathSecret, infinitePayURLToken: infinitePayURLToken}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /webhooks/c6/"+h.c6PathSecret, h.receiveC6)
	mux.HandleFunc("POST /webhooks/infinitepay/"+h.infinitePayURLToken, h.receiveInfinitePay)
	mux.HandleFunc("POST /v1/webhooks/register", h.register)
}

func (h *Handler) receiveC6(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = h.svc.ProcessInbound(r.Context(), "c6", body)
	if errors.Is(err, service.ErrUntrustedOrigin) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if err != nil {
		// Still ack with 200: processing failures are recorded internally
		// and shouldn't trigger aggressive redelivery from C6.
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) receiveInfinitePay(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := h.svc.ProcessInbound(r.Context(), "infinitepay", body); err != nil {
		// InfinitePay retries on 400; still ack 200 to avoid retry storms
		// from transient processing errors. Failures are logged/persisted.
		slog.Error("infinitepay webhook: process failed", "error", err)
	}
	w.WriteHeader(http.StatusOK)
}

type registerPayload struct {
	BaaS    string `json:"baas"`
	Service string `json:"service"`
	URL     string `json:"url"`
}

// register is an administrative endpoint (not part of the public client
// API) used once to subscribe our callback URL with the BaaS for a given
// service. It should be protected at the infra/network layer.
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var payload registerPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := h.svc.RegisterWebhook(r.Context(), payload.BaaS, domain.RegisterWebhookRequest{
		BaaS:    payload.BaaS,
		Service: domain.Service(payload.Service),
		URL:     payload.URL,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusOK)
}
