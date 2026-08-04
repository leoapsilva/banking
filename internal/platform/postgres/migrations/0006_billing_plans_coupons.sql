-- Billing: plan catalogue and coupons.
--
-- Until now the amount to charge was supplied by the API client on every
-- request. That splits pricing across two systems: the client decides the
-- value and the banking decides the billing cycle, so "what does this
-- customer pay next cycle" cannot be answered in one place.
--
-- From here on the price lives with the subscription: clients send a plan
-- code plus an optional coupon code, and billing resolves the amount.

CREATE TABLE billing_plans (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code         TEXT        NOT NULL UNIQUE,
    description  TEXT        NOT NULL,
    frequency    TEXT        NOT NULL CHECK (frequency IN ('MONTHLY', 'ANNUAL')),
    amount_cents BIGINT      NOT NULL CHECK (amount_cents > 0),
    currency     TEXT        NOT NULL DEFAULT 'BRL',
    active       BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Coupons are deliberately generic: percentage or fixed amount, restricted to
-- a set of frequencies, with a validity window and an optional redemption cap.
-- No launch-campaign rule is hardcoded here.
CREATE TABLE billing_coupons (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code                  TEXT        NOT NULL UNIQUE,
    percent_off           INT         CHECK (percent_off > 0 AND percent_off <= 100),
    amount_off_cents      BIGINT      CHECK (amount_off_cents > 0),
    applicable_frequencies TEXT[]     NOT NULL DEFAULT '{MONTHLY,ANNUAL}',
    duration              TEXT        NOT NULL DEFAULT 'ONCE'
                              CHECK (duration IN ('ONCE', 'REPEATING', 'FOREVER')),
    valid_from            TIMESTAMPTZ,
    valid_until           TIMESTAMPTZ,
    max_redemptions       INT         CHECK (max_redemptions > 0),
    redemptions_count     INT         NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Exactly one discount kind must be set.
    CONSTRAINT billing_coupons_one_discount CHECK (
        (percent_off IS NOT NULL AND amount_off_cents IS NULL) OR
        (percent_off IS NULL AND amount_off_cents IS NOT NULL)
    )
);

CREATE INDEX idx_billing_plans_code ON billing_plans(code) WHERE active;
CREATE INDEX idx_billing_coupons_code ON billing_coupons(code);

-- Record which plan and coupon a subscription was created under, so the
-- renewal amount is reproducible and coupon duration ('ONCE' vs 'FOREVER')
-- can be honoured on later cycles.
ALTER TABLE subscriptions
    ADD COLUMN plan_code   TEXT,
    ADD COLUMN coupon_code TEXT;

-- No plan rows are seeded: prices are a product decision, not a schema
-- decision. Register them per environment, e.g.
--   INSERT INTO billing_plans (code, description, frequency, amount_cents)
--   VALUES ('cores-annual', 'Cores — plano anual', 'ANNUAL', 29990);
