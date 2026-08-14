# Architecture

## Boundaries

```text
Browser -> Caddy -> Next.js
               -> Go API
Go API/Worker -> Face AI (internal)
Face AI PoC -> Qdrant (internal benchmark collections)
Future application data services: PostgreSQL, Redis, Qdrant, MinIO
```

- Web owns rendering and browser interaction.
- Go API will own public contracts, tenancy, permissions and orchestration.
- Face AI will own model loading and inference only.
- Photo worker will own asynchronous image processing.
- PostgreSQL, Redis and MinIO are declared for future integration but are not used by application code yet.
- The Face AI PoC has a benchmark-only Qdrant adapter. It creates dedicated collections, stores opaque face/photo and dataset/Event scope identifiers, and requires both scope filters on every search. It is not the production multi-tenant vector adapter.

## Dependency direction

Public clients never call Face AI directly. AI and worker services are internal to the Docker network. Business contracts begin under `/api/v1`; health checks remain outside the versioned prefix.

## Reuse policy

The existing GPL Face-Search and AGPL Immich projects are architectural references only. Their source is not copied into this scaffold. Any future model artifact must receive an independent commercial-license review.
