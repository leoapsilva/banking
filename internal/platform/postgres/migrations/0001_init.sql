-- Fase 1: checkout, card tokens, subscriptions, webhooks (C6 Bank)

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE checkouts (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    baas                   TEXT NOT NULL,
    provider_checkout_id   TEXT NOT NULL,
    external_reference_id  TEXT,
    amount_cents           BIGINT NOT NULL,
    currency               TEXT NOT NULL DEFAULT 'BRL',
    status                 TEXT NOT NULL,
    checkout_url           TEXT,
    payer_tax_id           TEXT,
    payer_name             TEXT,
    raw_create_response    JSONB,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (baas, provider_checkout_id)
);

CREATE TABLE card_tokens (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    baas                TEXT NOT NULL,
    provider_token      TEXT NOT NULL,
    customer_tax_id     TEXT NOT NULL,
    card_brand          TEXT,
    card_last_digits    TEXT,
    card_holder_name    TEXT,
    source_checkout_id  UUID REFERENCES checkouts(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (baas, provider_token)
);

CREATE TABLE subscriptions (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    baas                  TEXT NOT NULL,
    customer_tax_id       TEXT NOT NULL,
    customer_name         TEXT NOT NULL,
    customer_email        TEXT,
    amount_cents          BIGINT NOT NULL,
    currency              TEXT NOT NULL DEFAULT 'BRL',
    frequency             TEXT NOT NULL,
    installments_total    INT,
    installments_done     INT NOT NULL DEFAULT 0,
    interest_type         TEXT,
    card_token_id         UUID REFERENCES card_tokens(id),
    initial_checkout_id   UUID REFERENCES checkouts(id),
    status                TEXT NOT NULL,
    next_charge_date      DATE,
    consecutive_failures  INT NOT NULL DEFAULT 0,
    cancelled_at          TIMESTAMPTZ,
    cancelled_reason      TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_subscriptions_due ON subscriptions (status, next_charge_date);

CREATE TABLE subscription_charges (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id        UUID NOT NULL REFERENCES subscriptions(id),
    cycle_date              DATE NOT NULL,
    external_reference_id  TEXT NOT NULL,
    checkout_id             UUID REFERENCES checkouts(id),
    status                  TEXT NOT NULL,
    provider_response_code TEXT,
    provider_message       TEXT,
    raw_response            JSONB,
    attempted_at            TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (subscription_id, cycle_date)
);

CREATE TABLE webhook_registrations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    baas           TEXT NOT NULL,
    service        TEXT NOT NULL,
    url            TEXT NOT NULL,
    registered_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (baas, service)
);

CREATE TABLE webhook_events (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    baas                   TEXT NOT NULL,
    provider_external_id   TEXT NOT NULL,
    service                TEXT NOT NULL,
    status                 TEXT NOT NULL,
    event_date_time        TIMESTAMPTZ,
    raw_payload            JSONB NOT NULL,
    processing_status      TEXT NOT NULL DEFAULT 'RECEIVED',
    received_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at           TIMESTAMPTZ,
    UNIQUE (baas, provider_external_id, status, event_date_time)
);

CREATE INDEX idx_webhook_events_processing_status ON webhook_events (processing_status);
