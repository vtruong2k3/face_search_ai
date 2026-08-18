# Operational Runbooks

Operational guidance for running Face Search AI: how to read the health and
metrics surfaces, what each key signal means, and first-response steps for common
incidents.

## Privacy rule for all telemetry

Logs, metrics, and traces must never contain biometric or personal data. Metric
labels are always bounded and low-cardinality. The following are never emitted as
a metric label or logged as a value: raw user/organization/event/photo ids, public
tokens, signed URLs, object storage paths, credentials or tokens, face embeddings,
and image bytes. Correlation identifiers (request id, job id, photo id) are allowed
as **structured log fields** for tracing a single operation, but never as metric
labels because they are unbounded. When adding a signal, re-read this rule before
choosing any label or field.

## Health surfaces

Each service separates liveness from readiness on purpose:

- **Liveness** (`GET /health/live`) reflects only that the process is running. It
  performs no dependency calls. A failing liveness check means the process is dead
  or wedged and should be restarted.
- **Readiness** (`GET /health/ready`) reflects whether the service can currently do
  useful work. It returns `503` when a required dependency is unavailable, so
  orchestrators stop routing traffic until the service recovers.

| Service      | Liveness   | Readiness                                           |
| ------------ | ---------- | --------------------------------------------------- |
| Go API       | process up | probes Postgres, Redis, MinIO, Qdrant, Face AI      |
| Face AI      | process up | model runtime loaded (when the pipeline is enabled) |
| Photo worker | process up | connected to Redis and joined its consumer group    |

The API readiness body reports each dependency as `{ "ok": bool, "error": "unavailable" }`.
The per-dependency detail is deliberately reduced to a uniform token so the
publicly reachable `/health/ready` cannot leak a connection string or URL; the map
key already identifies which dependency is unhealthy, which is the actionable part.

## Metrics surfaces

`/metrics` is exposed by the API (`:8080`), Face AI (`:8001`), and the worker
(`:9100`). All three are internal to the Docker network and are **not** routed
through the public reverse proxy. Prometheus scrapes them (see
`infra/prometheus/prometheus.yml`) and is bound to loopback for local operator
access. Grafana is a documented follow-up and is intentionally not bundled to keep
the monitoring surface minimal; point a Grafana instance at the Prometheus data
source when richer dashboards are needed.

### Key API signals

- `http_requests_total{method,route,status_class}` and
  `http_request_duration_seconds{...}` — request volume and latency. `route` is the
  matched route template (for example `/api/v1/public/events/{publicToken}`), never
  a concrete id. `status_class` is `2xx`/`4xx`/`5xx`.
- `upload_operations_total{operation}` — upload lifecycle (`initiate`, `complete`,
  `abort`, `create`, `reprocess`).
- `search_requests_total{outcome}` and `search_request_duration_seconds` — public
  selfie search volume/latency by bounded outcome class (`ok`, `consent_required`,
  `face_count_zero`, `face_count_multiple`, `selfie_too_large`, `service_unavailable`,
  `not_found`, …). The selfie and any embedding are never observed.
- `download_decisions_total{decision,kind}` — controlled downloads by `allowed`/
  `denied` and `single`/`bulk`.
- `rate_limit_rejections_total{surface}` — rejections per protected surface
  (`auth`, `search`, `download`).
- `dependency_up{dependency}` (1 healthy / 0 unhealthy) and
  `dependency_check_duration_seconds{dependency,result}` — dependency readiness and
  probe latency.

### Key Face AI signals

- `face_ai_inference_requests_total{outcome}` and
  `face_ai_inference_duration_seconds` — inference volume/latency by outcome class
  (`ok`, `not_ready`, `payload_too_large`, `empty_image`, `validation_error`,
  `inference_error`).

### Key worker signals

- `photo_worker_jobs_processed_total{job_type}` and
  `photo_worker_job_duration_seconds{job_type}` — successful jobs and latency.
- `photo_worker_jobs_retried_total{job_type}` — retryable attempt failures.
- `photo_worker_jobs_failed_total{job_type,reason}` — failures by reason
  (`terminal`, `exhausted`, `parse_error`).
- `photo_worker_jobs_dead_lettered_total` — jobs routed to the dead-letter stream.

## First-response playbooks

### A dependency is down

Signal: `dependency_up{dependency="X"} == 0`, API `/health/ready` returns `503`,
`dependency unhealthy` warning logs with the dependency name and a latency figure.

1. Identify the dependency from the label/log field (`postgres`, `redis`, `minio`,
   `qdrant`, `face_ai`).
2. Check that dependency's own container health and logs. Do not expect a connection
   string in our logs by design; use the dependency name to locate it.
3. For Postgres, also confirm the migration/schema version matches
   `DATABASE_SCHEMA_VERSION` (readiness fails on a schema mismatch).
4. Once the dependency recovers, readiness returns to `200` automatically; no API
   restart is required.

### High error rate

Signal: rising `http_requests_total{status_class="5xx"}` or elevated
`search_request_duration_seconds` / `http_request_duration_seconds`.

1. Break down `http_requests_total` by `route` and `status_class` to locate the
   failing surface.
2. Correlate with `dependency_up` — most `5xx`/`service_unavailable` spikes trace to
   a degraded dependency.
3. Use structured logs filtered by `request_id` to trace individual failures; the
   request id is returned to clients in the `X-Request-ID` header.
4. If a single dependency is the cause, follow the dependency-down playbook.

### Queue backlog or DLQ growth

Signal: rising `photo_worker_jobs_retried_total`, `photo_worker_jobs_failed_total`,
or `photo_worker_jobs_dead_lettered_total`; Redis stream pending entries growing.

1. Check worker readiness (`:9100/health/ready`) and logs for
   `job_processing_attempt_failed` / `job_sent_to_dlq` (correlated by photo id in
   the log field, never in a metric).
2. A spike in `reason="terminal"` usually means malformed/oversized inputs failing
   validation deterministically — inspect the affected photos rather than retrying.
3. A spike in `reason="exhausted"` usually means a transient dependency (Face AI,
   Qdrant, MinIO, Postgres) is degraded — check `dependency_up` on the API and the
   dependency itself, then let autoclaim reprocess pending messages.
4. Drain/inspect the dead-letter stream (`DEAD_LETTER_STREAM`) once the root cause is
   fixed. Reprocessing is idempotent.

### Deletion did not fully purge

Signal: a deleted Photo/Event still has objects in MinIO or vectors in Qdrant.

1. Deletion is tombstone-first: the resource is already non-searchable and
   non-downloadable the moment it is tombstoned (enforced by DB state, not by the
   vector store). A lingering object/vector is a purge-lag or purge-failure issue,
   not a privacy exposure.
2. The purge runs as `photo.deletion.requested` / `event.deletion.requested`
   outbox messages consumed by the worker. Check outbox rows
   (`SELECT status, last_error_code FROM outbox_messages WHERE event_type LIKE '%deletion%'`)
   and the worker DLQ. A stuck message points at a degraded MinIO/Qdrant/Postgres;
   follow the dependency-down playbook, then let autoclaim reprocess. Purge steps
   are idempotent, so reprocessing is safe.

### Backups and restore

- Take backups with `scripts/backup/backup.sh`; verify recoverability with
  `scripts/backup/restore-drill.sh` on disposable infra. Procedures and the full
  variable list are in `docs/backup-restore.md`.
- Deleted data is recoverable only from a backup taken before the deletion.

### Retention pruning

- `scripts/retention-prune.sh` enforces the documented windows idempotently; run
  it on a daily schedule. It prunes only time-bounded session/audit/operational
  rows and never Photo/Event/faces/object data. Do not shorten
  `AUDIT_RETENTION_DAYS` below your compliance obligation.

### Rate-limit spikes

Signal: rising `rate_limit_rejections_total{surface}`.

1. Identify the surface (`auth`, `search`, `download`).
2. A spike on `auth` may indicate credential-stuffing; on `search`/`download` it may
   indicate scraping of a public Event. These are working as designed — the limiter
   is protecting the system.
3. If legitimate traffic is being throttled, tune the relevant env limits
   (`AUTH_RATE_LIMIT`, `SEARCH_RATE_LIMIT`, `DOWNLOAD_RATE_LIMIT` and their windows).
   Client address is derived from the leftmost `X-Forwarded-For` entry and is used
   only for coarse abuse control, never for authorization.
