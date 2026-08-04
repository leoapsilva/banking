package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	billinghandler "github.com/upwifi/banking/internal/billing/handler"
	billingrepo "github.com/upwifi/banking/internal/billing/repository"
	billingservice "github.com/upwifi/banking/internal/billing/service"
	boletohandler "github.com/upwifi/banking/internal/boleto/handler"
	boletorepo "github.com/upwifi/banking/internal/boleto/repository"
	boletoservice "github.com/upwifi/banking/internal/boleto/service"
	checkouthandler "github.com/upwifi/banking/internal/checkout/handler"
	checkoutrepo "github.com/upwifi/banking/internal/checkout/repository"
	checkoutservice "github.com/upwifi/banking/internal/checkout/service"
	paymentschedulinghandler "github.com/upwifi/banking/internal/paymentscheduling/handler"
	paymentschedulingrepo "github.com/upwifi/banking/internal/paymentscheduling/repository"
	paymentschedulingservice "github.com/upwifi/banking/internal/paymentscheduling/service"
	pixhandler "github.com/upwifi/banking/internal/pix/handler"
	pixrepo "github.com/upwifi/banking/internal/pix/repository"
	pixservice "github.com/upwifi/banking/internal/pix/service"
	"github.com/upwifi/banking/internal/platform/config"
	"github.com/upwifi/banking/internal/platform/cronworker"
	"github.com/upwifi/banking/internal/platform/httpserver"
	"github.com/upwifi/banking/internal/platform/logging"
	"github.com/upwifi/banking/internal/platform/postgres"
	"github.com/upwifi/banking/internal/platform/providerregistry"
	c6provider "github.com/upwifi/banking/internal/provider/c6"
	c6auth "github.com/upwifi/banking/internal/provider/c6/auth"
	c6client "github.com/upwifi/banking/internal/provider/c6/client"
	infinitepayprovider "github.com/upwifi/banking/internal/provider/infinitepay"
	infinitepayclient "github.com/upwifi/banking/internal/provider/infinitepay/client"
	webhookhandler "github.com/upwifi/banking/internal/webhook/handler"
	webhookrepo "github.com/upwifi/banking/internal/webhook/repository"
	webhookservice "github.com/upwifi/banking/internal/webhook/service"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logging.Setup(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := postgres.RunMigrations(ctx, pool); err != nil {
		return err
	}

	// --- C6 provider wiring ---
	mtlsHTTPClient, err := c6auth.NewTLSHTTPClient(cfg.C6MTLSCertPath, cfg.C6MTLSKeyPath)
	if err != nil {
		return err
	}
	tokenManager := c6auth.New(mtlsHTTPClient, cfg.C6BaseURL(), cfg.C6ClientID, cfg.C6ClientSecret)

	c6CheckoutClient := c6client.New(mtlsHTTPClient, tokenManager, cfg.C6BaseURL()+"/v1/checkouts", cfg.C6PartnerName, cfg.C6PartnerVersion)
	c6WebhookClient := c6client.New(mtlsHTTPClient, tokenManager, cfg.C6BaseURL()+"/v1/webhooks", cfg.C6PartnerName, cfg.C6PartnerVersion)
	c6BankSlipClient := c6client.New(mtlsHTTPClient, tokenManager, cfg.C6BaseURL()+"/v1/bank_slips", cfg.C6PartnerName, cfg.C6PartnerVersion)
	c6PixClient := c6client.New(mtlsHTTPClient, tokenManager, cfg.C6BaseURL()+"/v2/pix", cfg.C6PartnerName, cfg.C6PartnerVersion)
	c6SchedulePaymentsClient := c6client.New(mtlsHTTPClient, tokenManager, cfg.C6BaseURL()+"/v1/schedule_payments", cfg.C6PartnerName, cfg.C6PartnerVersion)

	c6CheckoutAdapter := c6provider.NewCheckoutAdapter(c6CheckoutClient)
	c6WebhookAdapter := c6provider.NewWebhookAdapter(c6WebhookClient)
	c6BoletoAdapter := c6provider.NewBoletoAdapter(c6BankSlipClient)
	c6PixAdapter := c6provider.NewPixAdapter(c6PixClient)
	c6PaymentSchedulingAdapter := c6provider.NewPaymentSchedulingAdapter(c6SchedulePaymentsClient)

	registry := providerregistry.New()
	providerregistry.Register(registry, "checkout", "c6", c6CheckoutAdapter)
	providerregistry.Register(registry, "webhook", "c6", c6WebhookAdapter)
	providerregistry.Register(registry, "boleto", "c6", c6BoletoAdapter)
	providerregistry.Register(registry, "pix", "c6", c6PixAdapter)
	providerregistry.Register(registry, "paymentscheduling", "c6", c6PaymentSchedulingAdapter)

	// --- InfinitePay provider wiring ---
	infinitePayHTTPClient := &http.Client{Timeout: 30 * time.Second}
	infinitePayClient := infinitepayclient.New(infinitePayHTTPClient, cfg.InfinitePayBaseURL)
	infinitePayCheckoutAdapter := infinitepayprovider.NewCheckoutAdapter(infinitePayClient, cfg.InfinitePayHandle, cfg.InfinitePayWebhookURL)
	infinitePayWebhookAdapter := infinitepayprovider.NewWebhookAdapter()

	providerregistry.Register(registry, "checkout", "infinitepay", infinitePayCheckoutAdapter)
	providerregistry.Register(registry, "webhook", "infinitepay", infinitePayWebhookAdapter)

	// --- Feature wiring ---
	checkoutRepository := checkoutrepo.New(pool)
	checkoutSvc := checkoutservice.New(registry, checkoutRepository)
	checkoutHandler := checkouthandler.New(checkoutSvc)

	webhookRepository := webhookrepo.New(pool)
	webhookSvc := webhookservice.New(registry, webhookRepository, webhookservice.ExpectedOrigin{
		ClientID:  cfg.C6ExpectedClientID,
		PartnerID: cfg.C6ExpectedPartnerID,
	})
	webhookHandlerInst := webhookhandler.New(webhookSvc, cfg.WebhookPathSecret, cfg.InfinitePayWebhookPathSecret)

	billingRepository := billingrepo.New(pool)
	billingSvc := billingservice.New(billingRepository, checkoutRepository, checkoutSvc)
	billingHandlerInst := billinghandler.New(billingSvc, cfg.InfinitePayPlanMonthlyURL)

	webhookSvc.SetCheckoutSink(billingSvc)

	boletoRepository := boletorepo.New(pool)
	boletoSvc := boletoservice.New(registry, boletoRepository)
	boletoHandlerInst := boletohandler.New(boletoSvc)
	webhookSvc.SetBankSlipSink(boletoSvc)

	pixRepository := pixrepo.New(pool)
	pixSvc := pixservice.New(registry, pixRepository)
	pixHandlerInst := pixhandler.New(pixSvc)

	paymentSchedulingRepository := paymentschedulingrepo.New(pool)
	paymentSchedulingSvc := paymentschedulingservice.New(registry, paymentSchedulingRepository)
	paymentSchedulingHandlerInst := paymentschedulinghandler.New(paymentSchedulingSvc)

	// --- HTTP server ---
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	checkoutHandler.Register(mux)
	webhookHandlerInst.Register(mux)
	billingHandlerInst.Register(mux)
	boletoHandlerInst.Register(mux)
	pixHandlerInst.Register(mux)
	paymentSchedulingHandlerInst.Register(mux)

	// Authentication wraps everything except /healthz and the provider
	// webhook paths — providers call those and cannot hold our key; they
	// carry their own secret in the URL and are validated separately.
	apiKeyExempt := []string{"/healthz", "/webhooks/"}
	if len(cfg.APIKeys) == 0 {
		slog.Warn("API_KEYS is empty: every route is anonymous. Do not run like this outside local development.")
	}

	handler := httpserver.RequestLogger(cfg.LogHTTPBody)(
		httpserver.APIKeyAuth(cfg.APIKeys, apiKeyExempt)(
			http.MaxBytesHandler(mux, cfg.MaxRequestBodyBytes),
		),
	)

	// Timeouts are mandatory, not tuning: without them a stalled client
	// holds a connection and its goroutine indefinitely. WriteTimeout
	// exceeds ReadTimeout because provider calls happen while writing.
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// --- Cron worker: recurring monthly billing cycles ---
	worker := cronworker.New()
	if err := worker.Schedule("charge-due-subscriptions", cfg.CronChargeInterval, billingSvc.ChargeDueSubscriptions); err != nil {
		return err
	}
	worker.Start()
	defer worker.Stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
