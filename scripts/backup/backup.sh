#!/usr/bin/env bash
# Take a backup of PostgreSQL, MinIO, and Qdrant into a timestamped directory
# under BACKUP_ROOT. Env-driven; no secrets are stored in this script. Run
# against a running stack:
#
#   set -a; . ./.env; set +a
#   ./scripts/backup/backup.sh
#
# Artifacts are written under BACKUP_ROOT/<timestamp>/. Treat them as sensitive:
# they contain tenant metadata, stored objects, and face vectors.

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=scripts/backup/common.sh
. "${SCRIPT_DIR}/common.sh"

require_docker

DEST="$(backup_all)"

log "backup complete: ${DEST}"
printf '%s\n' "${DEST}"
