# Postgres image for the banking service.
#
# Schema migrations are applied by the banking binary itself on startup (see
# internal/platform/postgres), so there is no run-migrations.sh here — unlike
# the Cores image, which migrates from this side.
#
# What this image does add is backup/restore. This database holds checkouts,
# subscriptions, coupons and card tokens: losing it means losing the record of
# who paid, and InfinitePay does not expose that back to us via API.

FROM postgres:16-alpine

# pgcrypto (used by migrations for gen_random_uuid()) ships with the
# postgres-contrib package, which postgres:16-alpine already includes.

# The backup/restore scripts keep the same shape as the ones in the Cores
# repository and rely on `set -o pipefail`, which BusyBox sh does not support.
# Without pipefail a pg_dump that dies mid-pipe still produces a valid, EMPTY
# .gz and the script reports success — the one failure mode a backup must
# never have. So we install bash instead of weakening the scripts.
RUN apk add --no-cache bash

COPY backup.sh /backup.sh
COPY restore.sh /restore.sh

RUN chmod +x /backup.sh /restore.sh
