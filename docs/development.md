# Development

## Port map

| Service | Port |
|---|---:|
| Caddy application entry | 8088 |
| Next.js native dev | 3000 |
| Go API | 8080 |
| Face AI internal | 8001 |
| MinIO console | 9001 |

PostgreSQL, Redis, Qdrant, MinIO API and Face AI are not exposed by Compose unless local debugging later requires it.

Real Qdrant adapter tests are opt-in. Run them from an environment that can reach Qdrant and set `FACE_AI_QDRANT_INTEGRATION_URL` (for example, `http://qdrant:6333` from the Compose network). Default unit tests use an injected fake and do not require infrastructure.

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
