# Architecture

## Boundaries

```text
Browser -> Caddy -> Next.js
               -> Go API
Go API/Worker -> Face AI (internal)
Go API/Worker -> PostgreSQL, Redis, Qdrant, MinIO
Face AI PoC -> Qdrant (internal benchmark collections)
```

- Web owns rendering and browser interaction. Its authenticated Event detail surface creates one tenant/Event-scoped Uppy instance, uses API-authorized multipart callbacks, uploads parts directly to signed MinIO URLs, and destroys in-memory upload state when the scope unmounts or changes.
- Go API will own public contracts, tenancy, permissions and orchestration.
- Face AI will own model loading and inference only.
- Photo worker will own asynchronous image processing.
- PostgreSQL has a versioned tenant-safe MVP schema and Compose migration gate. The Go API owns a pgx-backed store boundary with transaction orchestration, sanitized persistence errors, bounded pooling, and migration-version readiness checks; schema mutation remains owned by the external migration service.
- Authentication supports registration, login, atomic opaque refresh-token rotation, logout, and current-user lookup. Passwords use Argon2id, only refresh-token hashes are persisted, access tokens are short-lived HS256 JWTs, and browser refresh tokens use scoped HttpOnly SameSite cookies.
- Authorization keeps JWTs tenant-neutral and resolves active user, organization, and membership state from PostgreSQL on every protected organization request. Roles are `owner`, `admin`, `editor`, and `viewer`; permissions are defined in one fail-closed domain matrix. Foreign, inactive, and nonexistent organization scope receives the same non-enumerating response.
- Trusted user and tenant principals use typed request context. Safe bounded request IDs correlate responses, structured request logs, and authorization audit records. Audit metadata excludes credentials, tokens, signed URLs, paths, image bytes, embeddings, and biometric observations.
- Events are tenant-scoped for authenticated management and archived rather than physically deleted. Public Event access uses opaque server-generated tokens and a limited anonymous DTO; private, archived, expired, tokenless, and unknown Events are externally indistinguishable.
- Photos use a fail-closed lifecycle and server-owned opaque object keys. Authenticated Photo list/read/create/soft-delete/reprocess operations require organization, Event, and Photo scope; public DTOs never expose object paths or failure internals. Multipart orchestration persists scoped expiring sessions and signs only the exact MinIO object/upload/part tuple, while browsers send image bytes directly to private object storage and completion verifies the expected size, MIME metadata, and optional checksum.
- Customer downloads are controlled and derive authorization only from the opaque public Event token, the Event download policy, and result scope; organization membership is never consulted for the public flow. The download service re-resolves the Event scope on every request, requires downloads to be enabled, and confirms each requested photo belongs to the resolved Event and is in the READY state before signing access. It returns short-lived, object-scoped signed GET URLs — no grant token is minted, persisted, or returned, so there is no replayable capability beyond the single object link, which expires and cannot reach any other object, Event, or tenant. Unknown, private, archived, and expired Events and Events with downloads disabled are indistinguishable. Requests are bounded (a capped photo-id count), rate limited per token and client address, and audited via decision-level download records plus request-correlated audit records; no signed URL, object path, token, embedding, or image byte is logged or stored in audit metadata.
- Every future Event, Photo, search, export, worker, and vector query must require organization scope in addition to resource identifiers; database foreign keys are only a backstop and never replace tenant authorization.
- Redis is configured but application job publication remains pending. MinIO is integrated for direct signed multipart Photo ingestion, and Compose initializes its private bucket; production Qdrant integration remains pending.
- The Face AI PoC has a benchmark-only Qdrant adapter. It creates dedicated collections, stores opaque face/photo and dataset/Event scope identifiers, and requires both scope filters on every search. It is not the production multi-tenant vector adapter.

## Abuse and HTTP controls

Deliberate request, rate, and browser-policy controls are matched to each endpoint's sensitivity. All safe error responses reuse the shared `{code, message}` shape, carry the correlation request ID and API security headers, and never leak internal detail.

- Request body size: JSON endpoints (auth, events, photos, downloads) share a tight 1 MiB cap enforced by `http.MaxBytesReader`; the multipart selfie-search endpoint keeps its ~10 MiB selfie cap plus small multipart overhead. Oversized bodies are rejected with `413` before decoding (selfie oversize maps to the typed `selfie_too_large`).
- Timeouts: the HTTP server sets `ReadHeaderTimeout` (5s), `ReadTimeout` (bounds the whole request body, including the ~10 MiB selfie), `WriteTimeout` (set above the per-request timeout so a safe response can be written), and `IdleTimeout`. A per-request timeout middleware returns a safe `503` `request_timeout` instead of an abrupt connection close. All are env-configurable. Legitimate multipart streaming is unaffected because responses are small JSON documents and the read timeout is sized above the selfie limit.
- Rate limits: the shared in-memory fixed-window limiter (V4 defers clustered limiting to a shared store) is applied to the highest-risk surfaces. Authentication (`register`/`login`/`refresh`) is limited per client address; public selfie search — the most expensive and biometric surface — is limited per public token combined with client address; controlled downloads remain limited per token and address. Exhausted budgets return `429` `rate_limited` with no internal detail. Limits and windows are env-configurable, and a non-positive limit disables limiting. Client address is derived from the reverse proxy's leftmost `X-Forwarded-For` entry (falling back to the transport address) and is used only for coarse abuse control, never for authorization.
- CORS: a single configured web origin is allowed with credentials; foreign origins are rejected before the handler. Preflight advertises exactly the methods the app uses (`GET, POST, PATCH, DELETE, OPTIONS` — Event update/archive need `PATCH`/`DELETE`) and the `Authorization`/`Content-Type` headers, and is cached briefly.
- CSRF and cookies: the refresh token stays in a scoped (`/api/v1/auth`) `HttpOnly` `SameSite=Lax` cookie, so browsers do not attach it to cross-site state-changing requests; combined with the credentialed-origin allow-list this protects the refresh/logout flow against CSRF without a separate token. `Secure` is enabled in TLS deployments via `REFRESH_COOKIE_SECURE`.
- Security headers: the JSON API sets `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Cross-Origin-Resource-Policy: same-site`, and `Cache-Control: no-store` (API responses are never HTML, so no document CSP is applied there). The web app sets `X-Content-Type-Options`, `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`, a `Permissions-Policy` that grants `camera=(self)` (required for `getUserMedia` on the public selfie page) while denying other powerful features, and a conservative CSP (`base-uri 'self'; object-src 'none'; frame-ancestors 'none'`). The edge proxy (Caddy) adds HSTS at the TLS-termination point plus `nosniff`, `X-Frame-Options`, and `Referrer-Policy`, and proxies only the web app and the `/api` and `/health` surfaces — no internal service is exposed.
- Request IDs: every response — including CORS rejections, `429`s, `413`s, and timeout `503`s — carries a bounded `X-Request-ID` that correlates structured request logs and authorization audit records.

Decision — deferred full web CSP: the web CSP deliberately omits a restrictive `default-src`/`script-src`/`connect-src`. Next.js hydration relies on inline bootstrap scripts and styles, and the browser connects to env-driven API and MinIO origins (selfie POST, direct multipart upload to signed URLs, signed downloads). A complete allow-list or nonce-based CSP would break hydration, camera capture, uploads, or downloads unless origins are finalized and a nonce middleware is added; that hardening is deferred to a release-readiness task. The shipped directives still block clickjacking, base-tag injection, and legacy plugin/object embedding.

## Observability and operational health

Every service separates liveness from readiness deliberately. Liveness
(`/health/live`) reflects only that the process is running and performs no
dependency calls; readiness (`/health/ready`) reflects whether the service can do
useful work. The Go API readiness probes Postgres, Redis, MinIO, Qdrant, and Face
AI concurrently; Face AI readiness reflects model-runtime load; the worker readiness
reflects that it connected to Redis and joined its consumer group. The API readiness
body reduces each dependency's detail to a uniform `unavailable` token so the
publicly proxied endpoint cannot leak a connection string or URL — the dependency
name (the map key) is the actionable part.

Each service exposes an internal `/metrics` endpoint (API `:8080`, Face AI `:8001`,
worker `:9100`). None is routed through the public reverse proxy: Caddy proxies only
the web app and the `/api` and `/health` surfaces. A minimal, loopback-bound
Prometheus service scrapes them on the Docker network (`infra/prometheus/prometheus.yml`);
Grafana is a documented follow-up rather than a bundled dependency.

All telemetry is privacy-safe. Metric labels are bounded and low-cardinality only:
HTTP method, matched route template, status class; upload operation; search outcome
class; download decision/kind; rate-limit surface; dependency name and healthy/
unhealthy result; worker job type and failure reason. No label or logged value ever
carries a raw user, organization, event, photo, or token identifier, a signed URL,
an object path, a credential, a face embedding, or image bytes. Correlation
identifiers (request id, job id, photo id) appear only as structured-log fields for
tracing a single operation, never as metric labels. Dependency-check outcomes emit
sanitized, actionable signals: a `dependency_up` gauge, a check-latency histogram,
and a warning log carrying only the dependency name and a latency figure. Operational
runbooks for reading these signals and responding to dependency-down, high-error-rate,
queue-backlog/DLQ, and rate-limit-spike incidents live in `docs/runbooks.md`.

## Dependency direction

Public clients never call Face AI directly. AI and worker services are internal to the Docker network. Business contracts begin under `/api/v1`; health checks remain outside the versioned prefix.

## Reuse policy

The existing GPL Face-Search and AGPL Immich projects are architectural references only. Their source is not copied into this scaffold. Any future model artifact must receive an independent commercial-license review.
