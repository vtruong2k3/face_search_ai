#!/usr/bin/env bash
# End-to-end backup/restore drill on DISPOSABLE infrastructure.
#
# It brings up an isolated Compose stack (unique project name + private volumes),
# seeds known marker data into PostgreSQL, MinIO, and Qdrant, takes a full backup,
# simulates data loss by destroying the volumes, brings the stack back up empty,
# restores from the backup, and verifies every marker survived. The disposable
# stack and its volumes are always torn down on exit.
#
# Requirements: Docker + docker compose. Nothing here touches a production target;
# the project name and volumes are unique to this drill.
#
#   set -a; . ./.env; set +a
#   ./scripts/backup/restore-drill.sh

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(cd -- "${SCRIPT_DIR}/../.." && pwd)

# Force a disposable, uniquely named project and matching backup network before
# sourcing common.sh so every helper targets the drill stack, not any real one.
export COMPOSE_PROJECT_NAME="face-search-restore-drill-${$}"
export BACKUP_NETWORK="${COMPOSE_PROJECT_NAME}_app"
export POSTGRES_USER="${POSTGRES_USER:-drill_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-drill-password-only}"
export POSTGRES_DB="${POSTGRES_DB:-face_search_drill}"
export MINIO_ROOT_USER="${MINIO_ROOT_USER:-drill-minio}"
export MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD:-drill-minio-only}"
export MINIO_BUCKET="${MINIO_BUCKET:-face-search}"
export QDRANT_COLLECTION="${QDRANT_COLLECTION:-face-search-faces}"

WORK_DIR="$(mktemp -d)"
export BACKUP_ROOT="${WORK_DIR}/backups"

# shellcheck source=scripts/backup/common.sh
. "${SCRIPT_DIR}/common.sh"

require_docker

MARKER_EMAIL="drill-marker@example.test"

compose() {
  docker compose --project-directory "${ROOT_DIR}" --project-name "${COMPOSE_PROJECT_NAME}" "$@"
}

psql_drill() {
  docker run --rm --network "${BACKUP_NETWORK}" -i \
    -e PGPASSWORD="${POSTGRES_PASSWORD}" \
    "${POSTGRES_IMAGE}" \
    psql --set=ON_ERROR_STOP=1 --tuples-only --no-align \
      --host="${PG_HOST}" --port="${PG_PORT}" \
      --username="${POSTGRES_USER}" --dbname="${POSTGRES_DB}" "$@"
}

qdrant_curl() {
  docker run --rm --network "${BACKUP_NETWORK}" "${CURL_IMAGE}" "$@"
}

cleanup() {
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  rm -rf "${WORK_DIR}" || true
}
trap cleanup EXIT

log "phase 1: bring up disposable source stack"
compose up --detach --wait postgres minio qdrant
compose run --rm migrations
compose run --rm minio-init

log "phase 2: seed known marker data"
psql_drill <<SQL
INSERT INTO users (email, password_hash) VALUES ('${MARKER_EMAIL}', 'hash');
SQL

# Seed one MinIO object under a realistic key.
echo "drill-object" >"${WORK_DIR}/marker.txt"
docker run --rm --network "${BACKUP_NETWORK}" \
  -e MC_HOST_dst="http://${MINIO_ACCESS_KEY}:${MINIO_SECRET_KEY}@${MINIO_HOST}:${MINIO_PORT}" \
  -v "${WORK_DIR}:/seed" \
  "${MC_IMAGE}" \
  cp /seed/marker.txt "dst/${MINIO_BUCKET}/organizations/drill/events/drill/photos/drill/original"

# Seed a Qdrant collection with a single point.
qdrant_curl -sf -X PUT "http://${QDRANT_HOST}:${QDRANT_PORT}/collections/${QDRANT_COLLECTION}" \
  -H 'Content-Type: application/json' \
  -d '{"vectors":{"size":4,"distance":"Cosine"}}'
qdrant_curl -sf -X PUT "http://${QDRANT_HOST}:${QDRANT_PORT}/collections/${QDRANT_COLLECTION}/points?wait=true" \
  -H 'Content-Type: application/json' \
  -d '{"points":[{"id":1,"vector":[0.1,0.2,0.3,0.4],"payload":{"photo_id":"drill"}}]}'

log "phase 3: take backup"
BACKUP_DIR="$(backup_all)"

log "phase 4: simulate data loss (destroy volumes) and bring up empty stack"
compose down --volumes --remove-orphans
compose up --detach --wait postgres minio qdrant
compose run --rm minio-init

log "phase 5: restore from backup"
restore_postgres "${BACKUP_DIR}/postgres.dump"
restore_minio "${BACKUP_DIR}/minio"
restore_qdrant "${BACKUP_DIR}/qdrant.snapshot"

log "phase 6: verify markers survived the restore"
users_count="$(psql_drill --command "SELECT count(*) FROM users WHERE email = '${MARKER_EMAIL}'")"
[ "${users_count}" = "1" ] || die "postgres marker missing after restore (got '${users_count}')"

object_present="$(docker run --rm --network "${BACKUP_NETWORK}" \
  -e MC_HOST_dst="http://${MINIO_ACCESS_KEY}:${MINIO_SECRET_KEY}@${MINIO_HOST}:${MINIO_PORT}" \
  "${MC_IMAGE}" \
  stat "dst/${MINIO_BUCKET}/organizations/drill/events/drill/photos/drill/original" >/dev/null 2>&1 \
  && echo yes || echo no)"
[ "${object_present}" = "yes" ] || die "minio marker object missing after restore"

points_count="$(qdrant_curl -sf "http://${QDRANT_HOST}:${QDRANT_PORT}/collections/${QDRANT_COLLECTION}" \
  | sed -n 's/.*"points_count":\([0-9]*\).*/\1/p')"
[ "${points_count}" = "1" ] || die "qdrant marker vector missing after restore (got '${points_count}')"

log "restore drill PASSED: postgres, minio, and qdrant markers all recovered"
