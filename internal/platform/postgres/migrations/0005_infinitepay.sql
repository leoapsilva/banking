-- InfinitePay checkout provider: additional columns on checkouts.
-- payer_email/payer_phone/payer_address are generic (any future provider
-- that carries payer contact data benefits); the rest are payment
-- confirmation fields populated from the InfinitePay webhook payload.

ALTER TABLE checkouts
    ADD COLUMN payer_email        TEXT,
    ADD COLUMN payer_phone        TEXT,
    ADD COLUMN payer_address      JSONB,
    ADD COLUMN capture_method     TEXT,
    ADD COLUMN installments       INT,
    ADD COLUMN paid_amount_cents  BIGINT,
    ADD COLUMN receipt_url        TEXT,
    ADD COLUMN transaction_id     TEXT,
    ADD COLUMN paid_at            TIMESTAMPTZ;
