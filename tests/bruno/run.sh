#!/usr/bin/env bash
# Runs the Bruno integration suite against a running banking instance and
# writes a formatted Markdown report to tests/bruno/reports/latest.md.
#
# Skips webhooks/01_registrar_webhook_checkout.bru and
# pix/11_registrar_webhook_pix.bru by default since both have a persistent
# side effect on the C6 sandbox account's webhook configuration; pass
# --include-webhook-register to include them anyway.
set -uo pipefail

cd "$(dirname "$0")"

ENV_NAME="${BRUNO_ENV:-local}"
if [ -z "${C6_WEBHOOK_PATH_SECRET:-}" ]; then
  echo "Set C6_WEBHOOK_PATH_SECRET (same value as your .env) before running this script." >&2
  exit 1
fi
if [ -z "${C6_SANDBOX_PIX_KEY:-}" ]; then
  echo "Set C6_SANDBOX_PIX_KEY (the sandbox account's PIX key, from docs/c6/auth/email_c6.txt) before running this script." >&2
  exit 1
fi

PATHS=(
  health
  checkouts
  subscriptions
  boleto
  pix/01_criar_cobranca_imediata.bru
  pix/02_criar_cobranca_imediata_com_devedor.bru
  pix/03_criar_cobranca_sem_baas_deve_falhar.bru
  pix/04_criar_cobranca_imediata_sem_pix_key_deve_falhar.bru
  pix/05_consultar_cobranca_imediata.bru
  pix/06_listar_cobrancas_imediatas.bru
  pix/07_criar_cobranca_com_vencimento.bru
  pix/08_criar_cobranca_vencimento_sem_endereco_deve_falhar.bru
  pix/09_consultar_cobranca_vencimento.bru
  pix/10_listar_cobrancas_vencimento.bru
  payment-scheduling
  webhooks/02_receber_notificacao_checkout_pago.bru
  webhooks/03_notificacao_duplicada_idempotente.bru
)

if [ "${1:-}" = "--include-webhook-register" ]; then
  PATHS+=(webhooks/01_registrar_webhook_checkout.bru pix/11_registrar_webhook_pix.bru)
fi

mkdir -p reports

npx --yes @usebruno/cli run "${PATHS[@]}" -r \
  --env "$ENV_NAME" \
  --env-var "C6_WEBHOOK_PATH_SECRET=$C6_WEBHOOK_PATH_SECRET" \
  --env-var "C6_SANDBOX_PIX_KEY=$C6_SANDBOX_PIX_KEY" \
  --reporter-json reports/latest.json

# bru exits non-zero when assertions fail, but the JSON reporter is still
# written before that exit — keep generating the report regardless.
node scripts/format-report.js reports/latest.json reports/latest.md
cat reports/latest.md
