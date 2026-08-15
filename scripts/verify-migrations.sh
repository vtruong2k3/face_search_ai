#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
PROJECT_NAME="face-search-migration-verify-${$}"
export POSTGRES_USER="migration_test"
export POSTGRES_PASSWORD="migration-test-only"
export POSTGRES_DB="face_search_test"
export MINIO_ROOT_USER="migration-test-minio"
export MINIO_ROOT_PASSWORD="migration-test-minio-only"

compose() {
  docker compose --project-directory "$ROOT_DIR" --project-name "$PROJECT_NAME" "$@"
}

psql_exec() {
  compose exec -T postgres psql --set=ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" "$@"
}

expect_failure() {
  local sql=$1
  if psql_exec --command "$sql" >/dev/null 2>&1; then
    printf 'expected SQL statement to fail: %s\n' "$sql" >&2
    return 1
  fi
}

cleanup() {
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

compose up --detach --wait postgres
compose run --rm migrations

psql_exec <<'SQL'
DO $$
DECLARE
    expected text[] := ARRAY[
        'audit_records', 'auth_sessions', 'download_records', 'events', 'faces',
        'organization_memberships', 'organizations', 'outbox_messages', 'photos',
        'schema_migrations', 'searches', 'users'
    ];
    actual text[];
BEGIN
    SELECT array_agg(tablename ORDER BY tablename)
      INTO actual
      FROM pg_tables
     WHERE schemaname = 'public';
    IF actual IS DISTINCT FROM expected THEN
        RAISE EXCEPTION 'unexpected tables: %', actual;
    END IF;
END $$;

INSERT INTO users (id, email, password_hash) VALUES
    ('00000000-0000-0000-0000-000000000001', 'owner@example.test', 'hash-1'),
    ('00000000-0000-0000-0000-000000000002', 'other@example.test', 'hash-2');
INSERT INTO organizations (id, name, slug) VALUES
    ('10000000-0000-0000-0000-000000000001', 'First Organization', 'first'),
    ('10000000-0000-0000-0000-000000000002', 'Second Organization', 'second');
INSERT INTO organization_memberships (organization_id, user_id, role) VALUES
    ('10000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'owner');
INSERT INTO events (id, organization_id, name, created_by_user_id) VALUES
    ('20000000-0000-0000-0000-000000000001', '10000000-0000-0000-0000-000000000001', 'Test Event', '00000000-0000-0000-0000-000000000001');
INSERT INTO photos (id, organization_id, event_id, object_key, created_by_user_id) VALUES
    ('30000000-0000-0000-0000-000000000001', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', 'org/event/photo', '00000000-0000-0000-0000-000000000001');
INSERT INTO faces (organization_id, event_id, photo_id, face_index, vector_point_id) VALUES
    ('10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', '30000000-0000-0000-0000-000000000001', 0, '40000000-0000-0000-0000-000000000001');
INSERT INTO outbox_messages (organization_id, aggregate_type, aggregate_id, event_type, payload, idempotency_key) VALUES
    ('10000000-0000-0000-0000-000000000001', 'photo', '30000000-0000-0000-0000-000000000001', 'photo.queued', '{}', 'photo-queued-1');
INSERT INTO auth_sessions (user_id, refresh_token_hash, family_id, expires_at) VALUES
    ('00000000-0000-0000-0000-000000000001', 'refresh-hash', '50000000-0000-0000-0000-000000000001', now() + interval '1 day');
INSERT INTO searches (id, organization_id, event_id, status, consent_recorded) VALUES
    ('60000000-0000-0000-0000-000000000001', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', 'completed', true);
INSERT INTO download_records (organization_id, event_id, photo_id, search_id, kind, decision) VALUES
    ('10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', '30000000-0000-0000-0000-000000000001', '60000000-0000-0000-0000-000000000001', 'single', 'allowed');
INSERT INTO audit_records (organization_id, actor_user_id, action, resource_type, resource_id, outcome) VALUES
    ('10000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'event.create', 'event', '20000000-0000-0000-0000-000000000001', 'success');
SQL

expect_failure "INSERT INTO users (email, password_hash) VALUES ('owner@example.test', 'duplicate')"
expect_failure "INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ('10000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000002', 'superuser')"
expect_failure "INSERT INTO photos (organization_id, event_id, object_key, created_by_user_id) VALUES ('10000000-0000-0000-0000-000000000002', '20000000-0000-0000-0000-000000000001', 'cross-tenant', '00000000-0000-0000-0000-000000000002')"
expect_failure "INSERT INTO outbox_messages (organization_id, aggregate_type, aggregate_id, event_type, payload, idempotency_key) VALUES ('10000000-0000-0000-0000-000000000001', 'photo', '30000000-0000-0000-0000-000000000001', 'photo.queued', '{}', 'photo-queued-1')"
expect_failure "INSERT INTO searches (organization_id, event_id, status, consent_recorded) VALUES ('10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', 'started', false)"

compose run --rm migrations
compose run --rm --entrypoint /migrate migrations -path=/migrations -database="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable" down -all

if psql_exec --tuples-only --no-align --command "SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations'" | grep -qv '^0$'; then
  printf 'application tables remain after migration down\n' >&2
  exit 1
fi

compose run --rm migrations
psql_exec --tuples-only --no-align --command "SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename = 'users'" | grep -q '^1$'
printf 'migration verification passed\n'
