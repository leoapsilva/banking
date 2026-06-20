package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/upwifi/banking/internal/webhook/domain"
	"github.com/upwifi/banking/internal/webhook/service"
)

type Handler struct {
	svc        *service.Service
	pathSecret string
}

// New creates the webhook handler. pathSecret is embedded in the inbound
// route to give it some obscurity, since C6 does not document an HMAC
// signature we could otherwise verify.
func New(svc *service.Service, pathSecret string) *Handler {
	return &Handler{svc: svc, pathSecret: pathSecret}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /webhooks/c6/"+h.pathSecret, h.receiveC6)
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
