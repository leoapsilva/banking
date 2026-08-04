#!/bin/bash
# Dump the banking database, compressed, with local rotation.
#
# Run from the host as:  docker-compose exec -T postgres /backup.sh
#
# pipefail matters here: without it, a pg_dump that dies mid-pipe still lets
# gzip exit 0, producing a valid but EMPTY archive while the script reports
# success. That is why this image installs bash.
set -euo pipefail

DB_USER="${POSTGRES_USER:-banking}"
DB_NAME="${POSTGRES_DB:-banking}"
BACKUP_DIR="/backups"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_FILE="${BACKUP_DIR}/${DB_NAME}_${TIMESTAMP}.sql.gz"
KEEP_LAST=7

mkdir -p "$BACKUP_DIR"

echo "Criando backup: $BACKUP_FILE"
pg_dump -U "$DB_USER" --clean --if-exists "$DB_NAME" | gzip > "$BACKUP_FILE"

# A dump that produced almost nothing is a failed dump that happened to exit
# cleanly. Refuse it instead of rotating a good backup out in its favour.
MIN_BYTES=1024
ACTUAL_BYTES=$(stat -c%s "$BACKUP_FILE")
if [ "$ACTUAL_BYTES" -lt "$MIN_BYTES" ]; then
  echo "ERRO: backup com apenas ${ACTUAL_BYTES} bytes — suspeito de dump vazio. Arquivo removido."
  rm -f "$BACKUP_FILE"
  exit 1
fi

# Verify the archive is readable before trusting it. Catches corruption that
# a size check alone would miss.
if ! gzip -t "$BACKUP_FILE"; then
  echo "ERRO: arquivo .gz corrompido. Arquivo removido."
  rm -f "$BACKUP_FILE"
  exit 1
fi

SIZE=$(du -sh "$BACKUP_FILE" | cut -f1)
echo "Backup criado: $BACKUP_FILE ($SIZE)"

# Rotação: mantém apenas os últimos KEEP_LAST backups localmente.
# A retenção longa é responsabilidade do destino remoto (ver deploy.yml).
TOTAL=$(ls -t "${BACKUP_DIR}/${DB_NAME}_"*.sql.gz 2>/dev/null | wc -l)
if [ "$TOTAL" -gt "$KEEP_LAST" ]; then
  ls -t "${BACKUP_DIR}/${DB_NAME}_"*.sql.gz | tail -n +"$((KEEP_LAST + 1))" | xargs rm -f
  echo "Rotação: mantidos os últimos $KEEP_LAST backups (removidos $((TOTAL - KEEP_LAST)))"
fi
