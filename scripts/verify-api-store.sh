#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_NAME="face-search-api-store-verify-${$}"
export POSTGRES_USER="api_store_test"
export POSTGRES_PASSWORD="api-store-test-only"
export POSTGRES_DB="face_search_test"
export MINIO_ROOT_USER="api-store-test-minio"
export MINIO_ROOT_PASSWORD="api-store-test-minio-only"

compose() {
  docker compose --project-directory "$ROOT_DIR" --project-name "$PROJECT_NAME" "$@"
}

cleanup() {
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

compose up --wait postgres
compose run --rm migrations

POSTGRES_CONTAINER="$(compose ps -q postgres)"
POSTGRES_IP="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$POSTGRES_CONTAINER")"
export API_POSTGRES_INTEGRATION_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_IP}:5432/${POSTGRES_DB}?sslmode=disable"

go -C "$ROOT_DIR/apps/api" test ./internal/store/postgres -run Integration -count=1
