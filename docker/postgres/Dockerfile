# Thin wrapper around the official Postgres image. Schema migrations are
# applied by the banking binary itself on startup (see
# internal/platform/postgres), so this image only needs the base server —
# kept as its own Dockerfile (instead of referencing postgres:16-alpine
# directly in compose) so future init scripts/extensions have a clear home.

FROM postgres:16-alpine

# pgcrypto (used by migrations for gen_random_uuid()) ships with the
# postgres-contrib package, which postgres:16-alpine already includes.
