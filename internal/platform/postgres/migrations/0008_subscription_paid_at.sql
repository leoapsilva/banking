-- When a subscription is completed, the caller (Cores) needs to know until
-- when the access it paid for is valid — today COMPLETED is a terminal
-- status that never changes, so an annual subscription paid once grants
-- access forever, with no renewal ever enforced.
--
-- paid_at records the instant the subscription actually got paid (set by
-- Complete(), never backfilled from another field): the API derives
-- current_period_end from it (paid_at + 1 year for ANNUAL) instead of
-- treating COMPLETED as unconditional, permanent access.
ALTER TABLE subscriptions
    ADD COLUMN paid_at TIMESTAMPTZ;
