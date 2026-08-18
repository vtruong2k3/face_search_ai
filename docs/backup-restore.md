# Backup, Restore, and Retention

Operational procedures for backing up and recovering Face Search AI state, and
for enforcing data retention. All scripts are env-driven and store no secrets;
tool images are pinned to the same versions used by `docker-compose.yml`.

State lives in three stores, each backed up independently:

| Store      | Contents                                   | Tool (pinned)                     |
| ---------- | ------------------------------------------ | --------------------------------- |
| PostgreSQL | tenants, events, photos, faces, audit rows | `pg_dump` / `pg_restore` (custom) |
| MinIO      | photo originals and derivatives            | `mc mirror`                       |
| Qdrant     | face vectors + scope payloads              | snapshot API                      |

Backup artifacts contain tenant metadata, stored objects, and face vectors.
Treat them as sensitive: store them encrypted, off the app hosts, and outside
this repository. The scripts write to `BACKUP_ROOT` (default `./backups`, which
is git-ignored).

## Configuration

All scripts read configuration from the environment. Load your ignored `.env`
first:

```bash
set -a; . ./.env; set +a
```

Key variables (safe placeholders are in `.env.example`):

- `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
- `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD`, `MINIO_BUCKET`
- `QDRANT_COLLECTION`
- `BACKUP_ROOT` — directory for backup artifacts
- `BACKUP_NETWORK` — Docker network of the running stack (default
  `${COMPOSE_PROJECT_NAME:-face-search-ai}_app`)
- Retention windows (days): `SEARCH_RETENTION_DAYS`,
  `DOWNLOAD_RECORD_RETENTION_DAYS`, `AUDIT_RETENTION_DAYS`,
  `UPLOAD_SESSION_RETENTION_DAYS`, `OUTBOX_RETENTION_DAYS`

## Take a backup

```bash
./scripts/backup/backup.sh
```

Writes `BACKUP_ROOT/<timestamp>/{postgres.dump, minio/, qdrant.snapshot}` and
prints the directory path. Run it against a healthy, running stack. For a
crash-consistent database dump under load, prefer a quiescent window or a
storage-level snapshot; `pg_dump` gives a transactionally consistent logical
backup on its own.

## Restore

Restoring overwrites data in the **target** stack and is destructive by design.
Point it only at a disposable drill stack or an authorized recovery target.

```bash
./scripts/backup/restore.sh ./backups/<timestamp>
```

This runs `pg_restore --clean --if-exists`, mirrors objects back into the
bucket, and uploads the Qdrant snapshot with `priority=snapshot` (which recreates
the collection).

## Restore drill (disposable infrastructure)

The drill proves the backup/restore path works without touching any real
deployment. It uses a uniquely named Compose project and private volumes, and
always tears them down on exit.

```bash
./scripts/backup/restore-drill.sh
```

Steps performed:

1. Bring up a disposable stack (`postgres`, `minio`, `qdrant`) + run migrations.
2. Seed known marker data into each store.
3. Take a full backup.
4. Destroy the volumes (simulated data loss) and bring the stack up empty.
5. Restore from the backup.
6. Verify every marker survived (a PostgreSQL row, a MinIO object, a Qdrant
   vector). The script exits non-zero if any marker is missing.

A passing run prints `restore drill PASSED`. Run this on a machine with Docker
available; it is not part of the unit test suites because it requires live
containers.

## Retention pruning

```bash
./scripts/retention-prune.sh
```

Deletes only rows older than their documented window (see the table in
`docs/architecture.md`) and nothing else. It is idempotent and safe to run on a
schedule (for example a daily cron entry or systemd timer). It never deletes
Photo, Event, faces, or object data — that is handled by the deletion lifecycle.

Search-session rows still referenced by a retained download record are kept until
that download record is pruned, preserving audit linkage and foreign-key
integrity. Audit records are retained the longest and are never pruned before
`AUDIT_RETENTION_DAYS`.

## Verifying deletion (privacy guarantee)

After a Photo or Event is deleted:

- It is immediately non-searchable: public search filters candidates through a
  DB visibility check (`ready` state + active Event), so a tombstoned Photo or
  archived Event never appears in results even before its vectors are purged.
- It is immediately non-downloadable: download authorization requires the `ready`
  state and an active Event in SQL.
- Its underlying data is purged asynchronously and idempotently by the photo
  worker: MinIO objects (prefix-scoped), Qdrant vectors (tenant + resource
  filtered), and PostgreSQL face rows.

Deleted data can only be recovered from a backup taken before the deletion.
