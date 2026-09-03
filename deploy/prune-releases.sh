#!/usr/bin/env sh
set -eu

retain_days=${RETAIN_DAYS:-180}
case "$retain_days" in
  ''|*[!0-9]*)
    printf 'RETAIN_DAYS must be a positive integer\n' >&2
    exit 2
    ;;
esac
if [ "$retain_days" -lt 1 ]; then
  printf 'RETAIN_DAYS must be at least 1\n' >&2
  exit 2
fi

docker compose exec -T mysql sh -c 'exec mysql -h 127.0.0.1 -u root -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"' <<SQL
START TRANSACTION;
CREATE TEMPORARY TABLE prune_release_ids (id BIGINT PRIMARY KEY);
INSERT INTO prune_release_ids (id)
SELECT r.id
FROM releases r
WHERE r.created_at < DATE_SUB(UTC_TIMESTAMP(), INTERVAL ${retain_days} DAY)
  AND NOT EXISTS (
    SELECT 1 FROM release_operation_locks l
    WHERE l.release_id = r.id AND l.expires_at >= UTC_TIMESTAMP()
  )
  AND NOT EXISTS (
    SELECT 1 FROM release_projects p
    WHERE p.release_id = r.id AND p.status IN ('building', 'deploying')
  );
DELETE j FROM release_pipeline_jobs j
JOIN release_projects p ON p.id = j.release_project_id
JOIN prune_release_ids r ON r.id = p.release_id;
DELETE p FROM release_projects p JOIN prune_release_ids r ON r.id = p.release_id;
DELETE FROM release_events WHERE release_id IN (SELECT id FROM prune_release_ids);
DELETE FROM release_changes WHERE release_id IN (SELECT id FROM prune_release_ids);
DELETE FROM release_operation_locks WHERE release_id IN (SELECT id FROM prune_release_ids);
DELETE FROM releases WHERE id IN (SELECT id FROM prune_release_ids);
DROP TEMPORARY TABLE prune_release_ids;
COMMIT;
SQL

printf 'Release retention cleanup completed; retained at least %s days.\n' "$retain_days"
