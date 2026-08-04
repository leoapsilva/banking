#!/bin/bash
# Restore the banking database from a compressed dump.
#
#   docker-compose exec -T postgres /restore.sh                    # último backup
#   docker-compose exec -T postgres /restore.sh /backups/arq.sql.gz
#
# A backup that has never been restored is not a backup. Exercise this on a
# throwaway environment before you need it for real.
set -euo pipefail

DB_USER="${POSTGRES_USER:-banking}"
DB_NAME="${POSTGRES_DB:-banking}"
BACKUP_DIR="/backups"

BACKUP_FILE="${1:-}"
if [ -z "$BACKUP_FILE" ]; then
  BACKUP_FILE=$(ls -t "${BACKUP_DIR}/${DB_NAME}_"*.sql.gz 2>/dev/null | head -1 || true)
  if [ -z "$BACKUP_FILE" ]; then
    echo "ERRO: nenhum backup encontrado em ${BACKUP_DIR}"
    exit 1
  fi
  echo "Nenhum arquivo informado — usando o mais recente: $BACKUP_FILE"
fi

if [ ! -f "$BACKUP_FILE" ]; then
  echo "ERRO: arquivo não encontrado: $BACKUP_FILE"
  exit 1
fi

if ! gzip -t "$BACKUP_FILE"; then
  echo "ERRO: arquivo .gz corrompido: $BACKUP_FILE"
  exit 1
fi

# This overwrites live data. The dump was taken with --clean --if-exists, so
# it drops existing objects before recreating them.
echo "⚠️  Restaurando $BACKUP_FILE sobre a base '${DB_NAME}'."
echo "⚠️  Os dados atuais serão substituídos."
if [ "${RESTORE_CONFIRM:-}" != "yes" ]; then
  echo "Abortado. Para confirmar, execute com RESTORE_CONFIRM=yes."
  exit 1
fi

gunzip -c "$BACKUP_FILE" | psql -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1
echo "✓ Restore concluído a partir de $BACKUP_FILE"
