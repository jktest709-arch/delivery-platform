#!/usr/bin/env sh
set -eu

backup_dir=${BACKUP_DIR:-./backups}
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
backup_file="${backup_dir}/delivery-platform-${timestamp}.sql"

mkdir -p "$backup_dir"
docker compose exec -T mysql sh -c \
  'exec mysqldump --single-transaction --routines --events --triggers -h 127.0.0.1 -u root -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"' \
  > "$backup_file"

printf 'MySQL backup written to %s\n' "$backup_file"
