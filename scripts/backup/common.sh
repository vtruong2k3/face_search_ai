#!/usr/bin/env bash
# Shared configuration and helpers for the Face Search AI backup, restore, and
# restore-drill scripts. Source this file; do not execute it directly.
#
# All configuration comes from the environment (or an ignored `.env`). No secret
# is ever written into these scripts or committed. Tool images are pinned to the
# same versions used by docker-compose.yml so backups and restores are performed
# with a matching client and server.

set -euo pipefail

# Pinned tool images (match docker-compose.yml service versions).
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:17-alpine}"
MC_IMAGE="${MC_IMAGE:-minio/mc:RELEASE.2025-08-13T08-35-41Z}"
CURL_IMAGE="${CURL_IMAGE:-curlimages/curl:8.11.1}"

# Docker network the running stack is attached to. docker-compose names it
# "<project>_app"; override BACKUP_NETWORK to target a different deployment.
COMPOSE_PROJECT="${COMPOSE_PROJECT_NAME:-face-search-ai}"
BACKUP_NETWORK="${BACKUP_NETWORK:-${COMPOSE_PROJECT}_app}"

# Service hostnames on the app network.
PG_HOST="${PG_HOST:-postgres}"
PG_PORT="${PG_PORT:-5432}"
MINIO_HOST="${MINIO_HOST:-minio}"
MINIO_PORT="${MINIO_PORT:-9000}"
QDRANT_HOST="${QDRANT_HOST:-qdrant}"
QDRANT_PORT="${QDRANT_PORT:-6333}"

# Credentials and resource names (env-driven; safe placeholders live in .env.example).
POSTGRES_USER="${POSTGRES_USER:?POSTGRES_USER is required}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}"
POSTGRES_DB="${POSTGRES_DB:?POSTGRES_DB is required}"
MINIO_ACCESS_KEY="${MINIO_ROOT_USER:-${MINIO_ACCESS_KEY:?MINIO_ROOT_USER or MINIO_ACCESS_KEY is required}}"
MINIO_SECRET_KEY="${MINIO_ROOT_PASSWORD:-${MINIO_SECRET_KEY:?MINIO_ROOT_PASSWORD or MINIO_SECRET_KEY is required}}"
MINIO_BUCKET="${MINIO_BUCKET:-face-search}"
QDRANT_COLLECTION="${QDRANT_COLLECTION:-face-search-faces}"

# Where backup artifacts are written. Keep this outside the repository or in an
# ignored path; artifacts may contain tenant data.
BACKUP_ROOT="${BACKUP_ROOT:-./backups}"

log() { printf '[backup] %s\n' "$*" >&2; }
die() { printf '[backup] error: %s\n' "$*" >&2; exit 1; }

require_docker() {
  command -v docker >/dev/null 2>&1 || die "docker is required"
}

# pg_dump the entire database to a compressed custom-format archive.
backup_postgres() {
  local out="$1"
  log "dumping PostgreSQL database ${POSTGRES_DB}"
  docker run --rm --network "${BACKUP_NETWORK}" \
    -e PGPASSWORD="${POSTGRES_PASSWORD}" \
    -v "$(cd "$(dirname "${out}")" && pwd):/backup" \
    "${POSTGRES_IMAGE}" \
    pg_dump --format=custom --no-owner --no-privileges \
      --host="${PG_HOST}" --port="${PG_PORT}" --username="${POSTGRES_USER}" \
      --dbname="${POSTGRES_DB}" --file="/backup/$(basename "${out}")"
}

# Restore a custom-format archive into the target database. The target must be
# reachable and empty (or use --clean semantics). Fails closed on any error.
restore_postgres() {
  local archive="$1"
  log "restoring PostgreSQL database ${POSTGRES_DB}"
  docker run --rm --network "${BACKUP_NETWORK}" \
    -e PGPASSWORD="${POSTGRES_PASSWORD}" \
    -v "$(cd "$(dirname "${archive}")" && pwd):/backup" \
    "${POSTGRES_IMAGE}" \
    pg_restore --no-owner --no-privileges --clean --if-exists \
      --host="${PG_HOST}" --port="${PG_PORT}" --username="${POSTGRES_USER}" \
      --dbname="${POSTGRES_DB}" "/backup/$(basename "${archive}")"
}

# Mirror the MinIO bucket into a local directory (originals + derivatives).
backup_minio() {
  local out_dir="$1"
  mkdir -p "${out_dir}"
  log "mirroring MinIO bucket ${MINIO_BUCKET}"
  docker run --rm --network "${BACKUP_NETWORK}" \
    -e MC_HOST_src="http://${MINIO_ACCESS_KEY}:${MINIO_SECRET_KEY}@${MINIO_HOST}:${MINIO_PORT}" \
    -v "$(cd "${out_dir}" && pwd):/backup" \
    "${MC_IMAGE}" \
    mirror --overwrite --remove "src/${MINIO_BUCKET}" "/backup"
}

# Mirror a local directory back into the MinIO bucket.
restore_minio() {
  local in_dir="$1"
  log "restoring MinIO bucket ${MINIO_BUCKET}"
  docker run --rm --network "${BACKUP_NETWORK}" \
    -e MC_HOST_dst="http://${MINIO_ACCESS_KEY}:${MINIO_SECRET_KEY}@${MINIO_HOST}:${MINIO_PORT}" \
    -v "$(cd "${in_dir}" && pwd):/backup" \
    "${MC_IMAGE}" \
    sh -c "mc mb --ignore-existing \"dst/${MINIO_BUCKET}\" && mc mirror --overwrite \"/backup\" \"dst/${MINIO_BUCKET}\""
}

# Create and download a Qdrant snapshot of the faces collection.
backup_qdrant() {
  local out="$1"
  log "creating Qdrant snapshot for collection ${QDRANT_COLLECTION}"
  local base="http://${QDRANT_HOST}:${QDRANT_PORT}/collections/${QDRANT_COLLECTION}/snapshots"
  local name
  name=$(docker run --rm --network "${BACKUP_NETWORK}" "${CURL_IMAGE}" \
    -sf -X POST "${base}" \
    | sed -n 's/.*"name":"\([^"]*\)".*/\1/p')
  [ -n "${name}" ] || die "failed to create Qdrant snapshot"
  log "downloading Qdrant snapshot ${name}"
  docker run --rm --network "${BACKUP_NETWORK}" \
    -v "$(cd "$(dirname "${out}")" && pwd):/backup" \
    "${CURL_IMAGE}" \
    -sf -o "/backup/$(basename "${out}")" "${base}/${name}"
}

# Upload and recover a Qdrant snapshot into the collection.
restore_qdrant() {
  local snapshot="$1"
  log "restoring Qdrant snapshot into collection ${QDRANT_COLLECTION}"
  local url="http://${QDRANT_HOST}:${QDRANT_PORT}/collections/${QDRANT_COLLECTION}/snapshots/upload?priority=snapshot"
  docker run --rm --network "${BACKUP_NETWORK}" \
    -v "$(cd "$(dirname "${snapshot}")" && pwd):/backup" \
    "${CURL_IMAGE}" \
    -sf -X POST "${url}" -H 'Content-Type:multipart/form-data' \
    -F "snapshot=@/backup/$(basename "${snapshot}")"
}

# Take a full backup into a timestamped directory under BACKUP_ROOT and print the
# directory path on stdout (log lines go to stderr, so callers can capture it).
backup_all() {
  local stamp dest
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  dest="${BACKUP_ROOT%/}/${stamp}"
  mkdir -p "${dest}/minio"
  backup_postgres "${dest}/postgres.dump"
  backup_minio "${dest}/minio"
  backup_qdrant "${dest}/qdrant.snapshot"
  printf '%s\n' "${dest}"
}
