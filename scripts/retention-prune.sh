#!/usr/bin/env bash
# Idempotent retention pruning for time-bounded records. Deletes rows older than
# their documented retention window and nothing else. Safe to run repeatedly and
# on a schedule (e.g. a daily cron/systemd timer). It never touches Photo, Event,
# faces, or object data — deletion of those is handled by the deletion lifecycle.
#
# Retention windows (days) are env-driven with conservative defaults. Audit and
# download records are retained longer than ephemeral search-session records.
# Search-session rows still referenced by a retained download record are kept
# until that download record is itself pruned, preserving audit linkage and
# foreign-key integrity.
#
#   set -a; . ./.env; set +a
#   ./scripts/retention-prune.sh

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=scripts/backup/common.sh
. "${SCRIPT_DIR}/backup/common.sh"

require_docker

# Retention windows in whole days. Validated as integers to keep them safe to
# interpolate into the SQL interval expressions below.
SEARCH_RETENTION_DAYS="${SEARCH_RETENTION_DAYS:-90}"
DOWNLOAD_RECORD_RETENTION_DAYS="${DOWNLOAD_RECORD_RETENTION_DAYS:-365}"
AUDIT_RETENTION_DAYS="${AUDIT_RETENTION_DAYS:-365}"
UPLOAD_SESSION_RETENTION_DAYS="${UPLOAD_SESSION_RETENTION_DAYS:-7}"
OUTBOX_RETENTION_DAYS="${OUTBOX_RETENTION_DAYS:-7}"

for var in SEARCH_RETENTION_DAYS DOWNLOAD_RECORD_RETENTION_DAYS \
           AUDIT_RETENTION_DAYS UPLOAD_SESSION_RETENTION_DAYS OUTBOX_RETENTION_DAYS; do
  value="${!var}"
  case "${value}" in
    ''|*[!0-9]*) die "${var} must be a non-negative integer, got '${value}'" ;;
  esac
done

log "pruning retention (search=${SEARCH_RETENTION_DAYS}d download=${DOWNLOAD_RECORD_RETENTION_DAYS}d audit=${AUDIT_RETENTION_DAYS}d upload_session=${UPLOAD_SESSION_RETENTION_DAYS}d outbox=${OUTBOX_RETENTION_DAYS}d)"

docker run --rm --network "${BACKUP_NETWORK}" -i \
  -e PGPASSWORD="${POSTGRES_PASSWORD}" \
  "${POSTGRES_IMAGE}" \
  psql --set=ON_ERROR_STOP=1 \
    --host="${PG_HOST}" --port="${PG_PORT}" \
    --username="${POSTGRES_USER}" --dbname="${POSTGRES_DB}" <<SQL
BEGIN;

-- Compliance audit trail: retained the longest.
DELETE FROM audit_records
 WHERE created_at < now() - interval '${AUDIT_RETENTION_DAYS} days';

-- Download decision records: retained for the audit window.
DELETE FROM download_records
 WHERE created_at < now() - interval '${DOWNLOAD_RECORD_RETENTION_DAYS} days';

-- Ephemeral search-session records: shorter window. Keep any row still
-- referenced by a retained download record to preserve foreign-key integrity.
DELETE FROM searches s
 WHERE s.requested_at < now() - interval '${SEARCH_RETENTION_DAYS} days'
   AND NOT EXISTS (
     SELECT 1 FROM download_records d WHERE d.search_id = s.id
   );

-- Terminal upload sessions: short-lived operational rows.
DELETE FROM photo_upload_sessions
 WHERE status IN ('completed', 'aborted', 'expired')
   AND updated_at < now() - interval '${UPLOAD_SESSION_RETENTION_DAYS} days';

-- Published outbox rows: already delivered; safe to prune.
DELETE FROM outbox_messages
 WHERE status = 'published'
   AND coalesce(published_at, updated_at) < now() - interval '${OUTBOX_RETENTION_DAYS} days';

COMMIT;
SQL

log "retention prune complete"
