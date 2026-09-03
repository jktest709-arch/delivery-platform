#!/usr/bin/env sh
set -eu

backup_file=${1:-}
if [ -z "$backup_file" ] || [ ! -f "$backup_file" ]; then
  printf 'usage: CONFIRM_RESTORE=YES sh deploy/restore-mysql.sh path/to/backup.sql\n' >&2
  exit 2
fi
if [ "${CONFIRM_RESTORE:-}" != "YES" ]; then
  printf 'restore is destructive; set CONFIRM_RESTORE=YES to continue\n' >&2
  exit 2
fi

docker compose exec -T mysql sh -c \
  'exec mysql -h 127.0.0.1 -u root -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"' \
  < "$backup_file"

printf 'MySQL restore completed from %s\n' "$backup_file"
