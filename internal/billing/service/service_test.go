package service

import "testing"

// TestWithOrderNSU is the regression for the real bug found while testing
// the v0.18.0 payment flow with a live payment: InfinitePay's redirect
// never carries order_nsu (nor slug) — only transaction_id/transaction_nsu/
// capture_method — so the return page had no way to locate the checkout
// unless we embed order_nsu into the redirect_url ourselves.
func TestWithOrderNSU(t *testing.T) {
	cases := []struct {
		name        string
		redirectURL string
		orderNSU    string
		want        string
	}{
		{
			name:        "plain URL gets order_nsu appended",
			redirectURL: "https://use.cores.app.br/conta/concluido",
			orderNSU:    "ORDER-1",
			want:        "https://use.cores.app.br/conta/concluido?order_nsu=ORDER-1",
		},
		{
			name:        "existing query params are preserved",
			redirectURL: "https://use.cores.app.br/conta/concluido?utm_source=app",
			orderNSU:    "ORDER-1",
			want:        "https://use.cores.app.br/conta/concluido?order_nsu=ORDER-1&utm_source=app",
		},
		{
			name:        "special characters in order_nsu are escaped",
			redirectURL: "https://use.cores.app.br/conta/concluido",
			orderNSU:    "order id/with special&chars",
			want:        "https://use.cores.app.br/conta/concluido?order_nsu=order+id%2Fwith+special%26chars",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := withOrderNSU(tc.redirectURL, tc.orderNSU)
			if got != tc.want {
				t.Errorf("withOrderNSU(%q, %q) = %q, want %q", tc.redirectURL, tc.orderNSU, got, tc.want)
			}
		})
	}
}

func TestWithOrderNSU_InvalidURLReturnsUnchanged(t *testing.T) {
	invalid := "://not-a-valid-url"
	got := withOrderNSU(invalid, "ORDER-1")
	if got != invalid {
		t.Errorf("withOrderNSU(%q, ...) = %q, want unchanged %q", invalid, got, invalid)
	}
}

// TestCheckoutDescriptionOrDefault is the regression for a real complaint:
// buyers got a receipt e-mail saying just "Assinatura anual", with no
// mention of Cores — the plan's own catalogue description
// (billing_plans.description, e.g. "Cores — plano anual") must be what
// InfinitePay shows, not a generic hardcoded string.
func TestCheckoutDescriptionOrDefault(t *testing.T) {
	cases := []struct {
		name            string
		planDescription string
		want            string
	}{
		{name: "uses plan description when present", planDescription: "Cores — plano anual", want: "Cores — plano anual"},
		{name: "falls back to generic text when empty", planDescription: "", want: "Assinatura anual"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkoutDescriptionOrDefault(tc.planDescription)
			if got != tc.want {
				t.Errorf("checkoutDescriptionOrDefault(%q) = %q, want %q", tc.planDescription, got, tc.want)
			}
		})
	}
}
