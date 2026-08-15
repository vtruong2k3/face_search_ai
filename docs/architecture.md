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
- Authentication supports registration, login, atomic opaque refresh-token rotation, logout, and current-user lookup. Passwords use Argon2id, only refresh-token hashes are persisted, access tokens are short-lived HS256 JWTs, and browser refresh tokens use scoped HttpOnly SameSite cookies.
- Authorization keeps JWTs tenant-neutral and resolves active user, organization, and membership state from PostgreSQL on every protected organization request. Roles are `owner`, `admin`, `editor`, and `viewer`; permissions are defined in one fail-closed domain matrix. Foreign, inactive, and nonexistent organization scope receives the same non-enumerating response.
- Trusted user and tenant principals use typed request context. Safe bounded request IDs correlate responses, structured request logs, and authorization audit records. Audit metadata excludes credentials, tokens, signed URLs, paths, image bytes, embeddings, and biometric observations.
- Events are tenant-scoped for authenticated management and archived rather than physically deleted. Public Event access uses opaque server-generated tokens and a limited anonymous DTO; private, archived, expired, tokenless, and unknown Events are externally indistinguishable.
- Photos use a fail-closed lifecycle and server-owned opaque object keys. Authenticated Photo list/read/create/soft-delete/reprocess operations require organization, Event, and Photo scope; public DTOs never expose object paths or failure internals.
- Every future Event, Photo, search, export, worker, and vector query must require organization scope in addition to resource identifiers; database foreign keys are only a backstop and never replace tenant authorization.
- Redis and MinIO are configured dependencies; Compose initializes the MinIO bucket, while application job, object, and production Qdrant integrations remain pending.
- The Face AI PoC has a benchmark-only Qdrant adapter. It creates dedicated collections, stores opaque face/photo and dataset/Event scope identifiers, and requires both scope filters on every search. It is not the production multi-tenant vector adapter.

## Dependency direction

Public clients never call Face AI directly. AI and worker services are internal to the Docker network. Business contracts begin under `/api/v1`; health checks remain outside the versioned prefix.

## Reuse policy

The existing GPL Face-Search and AGPL Immich projects are architectural references only. Their source is not copied into this scaffold. Any future model artifact must receive an independent commercial-license review.
