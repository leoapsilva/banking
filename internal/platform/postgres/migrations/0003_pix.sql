-- Fase 2: PIX (cobrança imediata e com vencimento, webhook próprio do
-- produto PIX) — item 7 do roteiro de conformidade C6.

CREATE TABLE pix_charges (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    baas                     TEXT NOT NULL,
    txid                     TEXT NOT NULL,
    kind                     TEXT NOT NULL, -- IMMEDIATE | WITH_DUE_DATE
    amount_cents             BIGINT NOT NULL,
    pix_key                  TEXT NOT NULL,
    status                   TEXT NOT NULL,
    revision                 INT NOT NULL DEFAULT 0,
    location                 TEXT,
    expiration_seconds       INT,
    due_date                 DATE,
    debtor_document_type     TEXT,
    debtor_document_id       TEXT,
    debtor_name              TEXT,
    payer_request            TEXT,
    raw_create_response      JSONB,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (baas, txid)
);

CREATE INDEX idx_pix_charges_created_at ON pix_charges (baas, created_at);
CREATE INDEX idx_pix_charges_kind_created_at ON pix_charges (baas, kind, created_at);

CREATE TABLE pix_webhook_registrations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    baas           TEXT NOT NULL,
    pix_key        TEXT NOT NULL,
    url            TEXT NOT NULL,
    registered_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (baas, pix_key)
);
