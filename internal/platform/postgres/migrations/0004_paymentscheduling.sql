-- Fase 2: Agendamento de Pagamentos (DDA + grupos de pagamento) — item 8 do
-- roteiro de conformidade C6 (docs/c6/swagger/agendamento-de-pagamentos.yaml).
-- C6 is the source of truth for status; these tables are our audit trail.

CREATE TABLE payment_groups (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    baas                TEXT NOT NULL,
    provider_group_id   TEXT NOT NULL,
    uploader_name       TEXT,
    status              TEXT NOT NULL, -- SUBMITTED | APPROVAL_REQUESTED
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (baas, provider_group_id)
);

CREATE TABLE payment_group_items (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id            UUID NOT NULL REFERENCES payment_groups(id),
    provider_item_id    TEXT NOT NULL,
    content             TEXT NOT NULL,
    amount_cents        BIGINT NOT NULL,
    bank_code           TEXT,
    bank_name           TEXT,
    beneficiary_name    TEXT,
    description         TEXT,
    payer_name          TEXT,
    due_date            DATE,
    transaction_date    DATE,
    overdue             BOOLEAN,
    product_type        TEXT, -- BOLETO | PIX
    status               TEXT NOT NULL, -- READ_DATA | ERROR | DECODE_ERROR | PROCESSED | SCHEDULED | PROCESSING | SCHEDULING_CANCELLED
    error_message         TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (group_id, provider_item_id)
);

CREATE INDEX idx_payment_group_items_status ON payment_group_items (status);
