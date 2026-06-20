-- Fase 2: boleto bancário (cobrança) (C6 Bank)

CREATE TABLE bank_slips (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    baas                   TEXT NOT NULL,
    provider_bank_slip_id  TEXT NOT NULL,
    external_reference_id  TEXT,
    amount_cents           BIGINT NOT NULL,
    currency               TEXT NOT NULL DEFAULT 'BRL',
    status                 TEXT NOT NULL,
    due_date               DATE NOT NULL,
    payer_tax_id           TEXT,
    payer_name             TEXT,
    raw_create_response    JSONB,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (baas, provider_bank_slip_id)
);

CREATE INDEX idx_bank_slips_status ON bank_slips (status);
CREATE INDEX idx_bank_slips_due_date ON bank_slips (due_date);
