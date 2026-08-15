# Architecture

## Boundaries

```text
Browser -> Caddy -> Next.js
               -> Go API
Go API/Worker -> Face AI (internal)
Go API/Worker -> PostgreSQL, Redis, Qdrant, MinIO
Face AI PoC -> Qdrant (internal benchmark collections)
```

- Web owns rendering and browser interaction.
- Go API will own public contracts, tenancy, permissions and orchestration.
- Face AI will own model loading and inference only.
- Photo worker will own asynchronous image processing.
- PostgreSQL has a versioned tenant-safe MVP schema and Compose migration gate. The Go API owns a pgx-backed store boundary with transaction orchestration, sanitized persistence errors, bounded pooling, and migration-version readiness checks; schema mutation remains owned by the external migration service.
- Authentication supports registration, login, atomic opaque refresh-token rotation, logout, and current-user lookup. Passwords use Argon2id, only refresh-token hashes are persisted, access tokens are short-lived HS256 JWTs, and browser refresh tokens use scoped HttpOnly SameSite cookies. Tenant roles and permissions remain the Task 2.4 boundary.
- Redis and MinIO are configured dependencies; Compose initializes the MinIO bucket, while application job, object, and production Qdrant integrations remain pending.
- The Face AI PoC has a benchmark-only Qdrant adapter. It creates dedicated collections, stores opaque face/photo and dataset/Event scope identifiers, and requires both scope filters on every search. It is not the production multi-tenant vector adapter.

## Dependency direction

Public clients never call Face AI directly. AI and worker services are internal to the Docker network. Business contracts begin under `/api/v1`; health checks remain outside the versioned prefix.

## Reuse policy

The existing GPL Face-Search and AGPL Immich projects are architectural references only. Their source is not copied into this scaffold. Any future model artifact must receive an independent commercial-license review.
