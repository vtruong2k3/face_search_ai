# Development

## Port map

| Service | Port |
|---|---:|
| Caddy application entry | 8088 |
| Next.js native dev | 3000 |
| Go API | 8080 |
| Face AI internal | 8001 |
| MinIO console | 9001 |

PostgreSQL, Redis, the MinIO API, and Face AI are internal-only in Compose. Qdrant is bound to host loopback for opt-in local integration tests, and the MinIO console is exposed for local administration. None of these internal services is routed through Caddy.

Real Qdrant adapter tests are opt-in. Run them from an environment that can reach Qdrant and set `FACE_AI_QDRANT_INTEGRATION_URL` (for example, `http://qdrant:6333` from the Compose network). Default unit tests use an injected fake and do not require infrastructure.

## Database migrations

The versioned SQL migrations under `migrations/` are executed by a pinned one-shot Compose service. Normal startup waits for PostgreSQL migrations and idempotent creation of the configured MinIO bucket before starting the API. The API does not mutate the schema: its persistence adapter verifies that `schema_migrations` is clean and exactly matches `DATABASE_SCHEMA_VERSION` before reporting ready.

The Go PostgreSQL pool is bounded by `DATABASE_MAX_CONNECTIONS`. Repository adapters use the shared `DBTX` and transaction callback boundaries under `apps/api/internal/store`; callers do not own commits or rollbacks.

Copy `.env.example` to the ignored `.env` file and replace every local placeholder before starting the stack. Then use:

```bash
make compose-config
make migrate-up
make migrate-version
docker compose up --build
```

`make migrate-down` deliberately refuses to alter the normal development database. Use `make migrate-verify` to run the complete up/down/up cycle against an isolated Compose project and disposable PostgreSQL volume. Use `make api-store-verify` to apply the same migration to another isolated project and run the real Go persistence integration suite, including auth password/refresh hashing, duplicate registration rollback, refresh rotation/replay rejection, expiry, revocation, transaction behavior, constraint mapping, and schema readiness. Both verifications always remove their projects and volumes; neither touches the normal `postgres-data` volume.

## Authentication development

Set `AUTH_SIGNING_KEY` to at least 32 unpredictable bytes in the ignored `.env`; never use the example placeholder outside local configuration setup. `AUTH_ISSUER`, `AUTH_AUDIENCE`, access/refresh TTLs, `WEB_ORIGIN`, and `REFRESH_COOKIE_SECURE` configure token validation and the credentialed browser boundary. Set secure cookies to true when served over HTTPS.

The browser keeps access tokens in React memory only. It performs one refresh-cookie request during session restoration; tokens must not be added to localStorage, sessionStorage, URLs, or logs. Refresh and logout use a cookie scoped to `/api/v1/auth`, and credentialed CORS accepts only `WEB_ORIGIN`.

## Authorization development

Access tokens carry user identity but no organization role. Protected organization requests resolve current active membership from PostgreSQL, and actor identity always comes from validated bearer credentials rather than request fields. The centralized role/permission matrix fails closed for unknown values. Foreign, disabled, suspended, and nonexistent organization identifiers are intentionally non-enumerating.

The web organization selection is held in React memory only and is cleared on logout or rejected session restoration. Request IDs accept only 1–64 ASCII letters, digits, hyphens, or underscores; otherwise the API generates one and returns it as `X-Request-ID`. Authorization audit records contain only bounded action, resource, outcome, request ID, and allowlisted safe metadata. Never add credentials, tokens, refresh hashes, request bodies, signed URLs, storage paths, embeddings, image bytes, or biometric observations to logs or audits.

Every future repository, job, export, object operation, and vector search must accept organization ID as mandatory scope. A lookup by resource ID alone is not authorized even if foreign keys exist.

For a genuinely fresh local deployment, first confirm no needed local data exists, then explicitly remove the normal volumes:

```bash
docker compose down --volumes
docker compose up --build
```

Removing volumes is destructive and is never part of ordinary `make down`.

## Adding features

1. Define or update the contract in `packages/contracts/openapi.yaml`.
2. Add domain behavior under `apps/api/internal/domain`.
3. Add transport code under `apps/api/internal/http`.
4. Add model-specific code only inside `services/face-ai`.
5. Add asynchronous handlers only inside `workers/photo-worker`.
6. Add tests at each boundary before connecting external infrastructure.

## Conventions

- Configuration comes from environment variables.
- Public APIs are versioned.
- Internal services are not exposed through Caddy.
- IDs and tenant filters will be generated and enforced server-side.
- No production secret is committed.
