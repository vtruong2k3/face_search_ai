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

The versioned SQL migrations under `migrations/` are executed by a pinned one-shot Compose service. Normal startup waits for PostgreSQL migrations and idempotent creation of the configured MinIO bucket before starting the API. The API does not own runtime migration logic yet.

Copy `.env.example` to the ignored `.env` file and replace every local placeholder before starting the stack. Then use:

```bash
make compose-config
make migrate-up
make migrate-version
docker compose up --build
```

`make migrate-down` deliberately refuses to alter the normal development database. Use `make migrate-verify` to run the complete up/down/up cycle against an isolated Compose project and disposable PostgreSQL volume. The verification always removes that project and volume; it does not touch the normal `postgres-data` volume.

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
