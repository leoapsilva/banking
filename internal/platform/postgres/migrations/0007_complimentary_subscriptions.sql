-- Courtesy (complimentary) subscriptions.
--
-- Some customers are promised free access: a waived annual fee, a lifetime
-- exemption, or a goodwill grant made by support. None of that can be
-- expressed as a coupon — a 100%% discount would mean a zero-value charge,
-- which providers reject and which hides a deliberate commercial decision
-- inside pricing arithmetic.
--
-- A courtesy subscription never touches a payment provider: no checkout, no
-- card, no charge. It is active because someone decided it is.

ALTER TABLE subscriptions
    -- When set, billing never charges this subscription and the recurring
    -- worker skips it. NULL means "not a courtesy subscription".
    ADD COLUMN complimentary_until TIMESTAMPTZ,

    -- TRUE for open-ended exemptions ("vitalícia"), where complimentary_until
    -- is meaningless. Kept separate from a far-future date so the intent is
    -- readable in the data rather than inferred from the year 9999.
    ADD COLUMN complimentary_forever BOOLEAN NOT NULL DEFAULT FALSE,

    -- Why the exemption was granted, and by whom. Free access is the kind of
    -- decision that gets questioned months later.
    ADD COLUMN complimentary_reason TEXT,
    ADD COLUMN complimentary_granted_by TEXT,
    ADD COLUMN complimentary_granted_at TIMESTAMPTZ;

-- Find active courtesy grants cheaply when reporting who is not paying.
CREATE INDEX idx_subscriptions_complimentary ON subscriptions (complimentary_until)
    WHERE complimentary_until IS NOT NULL OR complimentary_forever;
