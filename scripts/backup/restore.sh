#!/usr/bin/env bash
# Restore PostgreSQL, MinIO, and Qdrant from a backup directory produced by
# backup.sh. This overwrites data in the target stack, so it is destructive by
# design — point it only at a target you intend to restore (a disposable drill
# stack or an authorized recovery target).
#
#   set -a; . ./.env; set +a
#   ./scripts/backup/restore.sh ./backups/<timestamp>

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=scripts/backup/common.sh
. "${SCRIPT_DIR}/common.sh"

require_docker

SRC="${1:?usage: restore.sh <backup-directory>}"
[ -d "${SRC}" ] || die "backup directory not found: ${SRC}"

restore_postgres "${SRC}/postgres.dump"
restore_minio "${SRC}/minio"
restore_qdrant "${SRC}/qdrant.snapshot"

log "restore complete from: ${SRC}"
