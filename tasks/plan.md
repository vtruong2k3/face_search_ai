# Implementation Plan: Face Search AI PoC and MVP

## Overview

Build the product in two gated stages: first prove face-search quality on an authorized dataset, then implement the tenant-isolated SaaS MVP. The AI benchmark is a mandatory decision gate. Payment, teams, clustering, branding, custom domains, GPU pools, and distributed scaling remain deferred.

## Architecture decisions

- Keep Next.js, Go API, Python Face AI, and Python worker as separate ownership boundaries.
- Treat `packages/contracts/openapi.yaml` as the public API source of truth.
- Store metadata in PostgreSQL, jobs in Redis, objects in MinIO, and vectors plus scoped metadata in Qdrant.
- Never persist customer selfie bytes by default; every vector query must include tenant and Event scope.
- Use neighboring projects only as read-only architectural references; independently implement all code.

## Phase 0 — Governance and reproducibility

### Task 0.1: Materialize the execution plan

**Description:** Create durable planning documents that can drive implementation across sessions.

**Acceptance criteria:**
- [x] Detailed task cards exist in `tasks/plan.md`.
- [x] A compact status checklist exists in `tasks/todo.md`.
- [x] V2–V4 features are explicitly deferred.

**Verification:**
- [x] Every implementation task has acceptance criteria, verification, dependencies, likely files, and S/M scope.

**Dependencies:** None

**Files likely touched:** `tasks/plan.md`, `tasks/todo.md`

**Estimated scope:** Small

### Task 0.2: Establish AI data and license controls

**Description:** Prevent unauthorized or accidental processing/commit of biometric data and unreviewed model artifacts.

**Acceptance criteria:**
- [ ] Dataset contract documents consent, lawful use, local-only paths, immutable enrollment/query split, and forbidden tracked artifacts.
- [ ] Manifest/config template contains opaque IDs and relative paths only, plus model provenance/checksum/license-review fields.
- [ ] Git ignores datasets, weights, embeddings, and benchmark outputs by default.

**Verification:**
- [ ] Review documentation against root `AGENTS.md`.
- [ ] Run a tracked/untracked artifact scan and confirm no sensitive data or model weight is present.

**Dependencies:** Task 0.1

**Files likely touched:** `.gitignore`, `docs/ai-benchmark.md`, `services/face-ai/config/benchmark.example.yaml`

**Estimated scope:** Medium

### Checkpoint 0

- [ ] Governance documentation agrees with `AGENTS.md`.
- [ ] No dataset, biometric output, secret, or unapproved model weight is present.

## Phase 1 — AI feasibility PoC

### Task 1.1: Define stable AI domain interfaces

**Description:** Introduce model-independent detector/embedder contracts, typed results, image validation, and deterministic vector normalization.

**Acceptance criteria:**
- [ ] Detection, aligned-face, embedding, and search-result types do not depend on a specific model runtime.
- [ ] Validation rejects excessive, malformed, unsupported, or unsafe image inputs.
- [ ] L2 normalization and result ordering are deterministic and covered by synthetic tests.

**Verification:**
- [ ] `uv run --project services/face-ai pytest`

**Dependencies:** Checkpoint 0

**Files likely touched:** `services/face-ai/src/face_ai/domain.py`, `pipeline.py`, `validation.py`, `services/face-ai/tests/`

**Estimated scope:** Medium

### Task 1.2: Add model lifecycle and ONNX provider selection

**Description:** Make model loading explicit, observable, replaceable, and safe for CPU-first development.

**Acceptance criteria:**
- [ ] Configuration declares model paths, checksums, provider, and whether artifacts are required.
- [ ] Readiness distinguishes optional, required-missing, and successfully loaded models without leaking local paths.
- [ ] Detector/embedder sessions are injectable in tests.

**Verification:**
- [ ] `uv run --project services/face-ai pytest`

**Dependencies:** Task 1.1

**Files likely touched:** `services/face-ai/src/face_ai/settings.py`, `runtime.py`, `main.py`, focused tests

**Estimated scope:** Medium

### Task 1.3: Implement detection, alignment, and embedding

**Description:** Implement the licensed SCRFD-compatible and ArcFace-compatible CPU pipeline behind the stable interfaces.

**Acceptance criteria:**
- [ ] Pipeline supports zero, one, and multiple detected faces with deterministic ordering.
- [ ] Landmark alignment, preprocessing, embedding, and normalization assumptions are documented and tested.
- [ ] Corrupt input and inference errors return typed failures without retaining image data.

**Verification:**
- [ ] `uv run --project services/face-ai pytest`
- [ ] Run the documented CPU smoke command on one authorized image.

**Dependencies:** Tasks 1.1, 1.2 and approved model license

**Files likely touched:** `services/face-ai/src/face_ai/models/`, `pipeline.py`, focused tests, `docs/ai-benchmark.md`

**Estimated scope:** Medium

### Task 1.4: Add benchmark Qdrant adapter

**Description:** Add a replaceable vector-index port and a dedicated, event/dataset-filtered Qdrant implementation for PoC experiments.

**Acceptance criteria:**
- [ ] Adapter creates, batches, searches, filters, and tears down a benchmark collection.
- [ ] Search requires a dataset/Event partition and stores no image bytes.
- [ ] Vector dimension and distance metric are validated.

**Verification:**
- [ ] Unit tests pass with a fake adapter.
- [ ] Qdrant integration test passes against Compose.

**Dependencies:** Task 1.1

**Files likely touched:** `services/face-ai/src/face_ai/vector_store.py`, `qdrant_store.py`, integration tests

**Estimated scope:** Medium

### Task 1.5: Build reproducible benchmark runner

**Description:** Validate a manifest, index enrollment photos, search query selfies, and emit privacy-safe aggregate metrics.

**Acceptance criteria:**
- [ ] Runner freezes config, seed, split, and model checksum and rejects enrollment/query leakage.
- [ ] Report computes precision, recall, FAR, FRR, Top-K, no-face rate, and latency percentiles using opaque IDs.
- [ ] Outputs are written only to ignored paths and contain no image bytes or direct identities.

**Verification:**
- [ ] A synthetic benchmark produces mathematically known metrics.
- [ ] Repeated runs with identical inputs produce identical logical results.

**Dependencies:** Tasks 1.3, 1.4

**Files likely touched:** `services/face-ai/src/face_ai/benchmark/`, tests, `docs/ai-benchmark.md`

**Estimated scope:** Medium

### Task 1.6: Calibrate threshold and report decision

**Description:** Sweep thresholds on the authorized dataset and prepare evidence for the user's AI viability decision.

**Acceptance criteria:**
- [ ] Report shows threshold trade-offs and available condition slices without identity exposure.
- [ ] Recommended threshold remains configuration, not a universal hard-coded constant.
- [ ] Model/license decision and CPU latency/throughput are recorded.

**Verification:**
- [ ] User reviews the frozen report and explicitly accepts or rejects MVP quality targets.

**Dependencies:** Task 1.5 and user-provided authorized dataset

**Files likely touched:** ignored benchmark output, `docs/ai-benchmark.md`, benchmark config

**Estimated scope:** Small

### Checkpoint 1 — AI viability gate

- [ ] Dataset and model approvals are complete.
- [ ] Accuracy and CPU performance metrics are reproducible.
- [ ] User explicitly approves proceeding to SaaS implementation.

## Phase 2 — MVP platform foundation

### Task 2.1: Harden infrastructure and add MVP migrations

**Description:** Make local dependencies repeatable and introduce only the schema required for MVP flows.

**Acceptance criteria:**
- [ ] Compose dependencies are pinned, persistent, healthy, and initialized without committed secrets.
- [ ] Repeatable migrations cover users, organizations/memberships, events, photos, faces, jobs/outbox, searches, downloads, sessions, and audit records.
- [ ] Constraints and indexes enforce valid states and tenant-scoped uniqueness.

**Verification:**
- [ ] Fresh Compose start and readiness pass.
- [ ] Migration up/down passes on disposable data.
- [ ] `docker compose config`

**Dependencies:** Checkpoint 1

**Files likely touched:** `docker-compose.yml`, `migrations/`, `Makefile`, `.env.example`

**Estimated scope:** Medium

### Task 2.2: Add Go persistence boundaries

**Description:** Wire PostgreSQL while preserving domain independence from SQL and HTTP.

**Acceptance criteria:**
- [ ] Pool/config, transaction helper, repository ports, and migration runner are available through dependency wiring.
- [ ] Failed operations roll back and unavailable database errors are mapped safely.
- [ ] Integration tests use disposable data.

**Verification:**
- [ ] `cd apps/api && go test ./...`

**Dependencies:** Task 2.1

**Files likely touched:** `apps/api/internal/config/config.go`, `internal/platform/`, `internal/store/`, tests

**Estimated scope:** Medium

### Task 2.3: Implement authentication slice

**Description:** Deliver register, login, refresh rotation, logout, and current-user behavior from OpenAPI through web UI.

**Acceptance criteria:**
- [ ] Contract and generated web types cover all auth operations.
- [ ] Passwords are hashed; access tokens are short-lived; refresh sessions rotate and revoke safely.
- [ ] Browser flow works with generic failure responses and no secret/token logging.

**Verification:**
- [ ] Contract lint/generation, Go auth tests, web tests, and Playwright auth flow pass.

**Dependencies:** Task 2.2

**Files likely touched:** `packages/contracts/openapi.yaml`, `apps/api/internal/domain/auth/`, auth handlers, web auth routes/components

**Estimated scope:** Medium

### Task 2.4: Implement tenant authorization foundation

**Description:** Centralize organization membership and permission enforcement before adding business resources.

**Acceptance criteria:**
- [ ] OWNER/ADMIN/EDITOR/VIEWER permissions are centralized and server-enforced.
- [ ] Request principal and audit context propagate safely.
- [ ] Two-tenant tests deny cross-tenant reads, writes, and ID enumeration.

**Verification:**
- [ ] `cd apps/api && go test ./...`

**Dependencies:** Task 2.3

**Files likely touched:** `apps/api/internal/domain/authorization/`, middleware, repositories, tests

**Estimated scope:** Medium

### Checkpoint 2

- [ ] Fresh deployment, migrations, and auth flow work.
- [ ] Automated tests prove tenant isolation.

## Phase 3 — Event management

### Task 3.1: Implement Event contract and domain

**Description:** Add tenant-scoped Event CRUD and processing status using trusted counters and server-generated identifiers.

**Acceptance criteria:**
- [ ] OpenAPI defines CRUD/status, visibility, expiry, permissions, threshold policy, counters, and valid states.
- [ ] Domain validates changes and ignores client attempts to set trusted counters/ownership.
- [ ] Cross-tenant access is denied.

**Verification:**
- [ ] OpenAPI lint and Go unit/integration tests pass.

**Dependencies:** Checkpoint 2

**Files likely touched:** contract, Event domain/repository/handlers, migrations/tests

**Estimated scope:** Medium

### Task 3.2: Build photographer Event UI

**Description:** Add authenticated Event list, creation, detail, and settings using generated API types.

**Acceptance criteria:**
- [ ] UI covers loading, empty, validation, success, and safe error states.
- [ ] Create/view/edit use generated contracts rather than independent request shapes.
- [ ] Relevant shipped Next.js 16 documentation is followed.

**Verification:**
- [ ] Vitest tests and Playwright create→view→edit pass.

**Dependencies:** Task 3.1 contract

**Files likely touched:** `apps/web/src/app/`, `components/`, `lib/api.ts`, tests

**Estimated scope:** Medium

### Task 3.3: Add public Event and QR

**Description:** Expose eligible Events through non-enumerable links and generate a QR to the canonical public URL.

**Acceptance criteria:**
- [ ] Public/private/expired policies are enforced server-side.
- [ ] Public token grants only configured Event capabilities.
- [ ] QR resolves to the correct customer landing page.

**Verification:**
- [ ] Go policy tests and Playwright QR target test pass.

**Dependencies:** Tasks 3.1, 3.2

**Files likely touched:** Event public contract/handler, web public Event route, QR component, tests

**Estimated scope:** Medium

### Checkpoint 3

- [ ] Photographer manages only owned/authorized Events.
- [ ] Customer opens only eligible public Events.

## Phase 4 — Secure photo ingestion

### Task 4.1: Define Photo lifecycle and object policy

**Description:** Establish valid photo transitions and non-enumerable tenant/Event-scoped object keys.

**Acceptance criteria:**
- [ ] States and transitions cover upload through READY/FAILED.
- [ ] List/delete/reprocess/status operations enforce tenant permissions.
- [ ] Cleanup operations are idempotent.

**Verification:**
- [ ] State-machine and authorization tests pass.

**Dependencies:** Checkpoint 3

**Files likely touched:** contract, Photo domain/repository/handlers, migrations/tests

**Estimated scope:** Medium

### Task 4.2: Implement signed multipart upload

**Description:** Let browsers upload directly to MinIO through short-lived, API-orchestrated multipart URLs.

**Acceptance criteria:**
- [ ] Initiate/sign-part/complete/abort operations never expose permanent credentials.
- [ ] Completion verifies storage metadata, type, size, and configured limits.
- [ ] Wrong-tenant, expired, invalid, incomplete, and aborted cases are rejected safely.

**Verification:**
- [ ] MinIO integration suite passes.

**Dependencies:** Task 4.1

**Files likely touched:** upload contracts/domain/handlers, MinIO adapter, tests

**Estimated scope:** Medium

### Task 4.3: Build resumable uploader UI

**Description:** Use existing Uppy packages for controlled multipart concurrency, retry, pause/resume, and progress.

**Acceptance criteria:**
- [ ] Per-file and aggregate progress plus recoverable errors are shown.
- [ ] Pause/resume and retry survive expected transient failures.
- [ ] Browser talks only to API orchestration and signed MinIO URLs.

**Verification:**
- [ ] Web tests and Playwright upload/retry/resume pass.

**Dependencies:** Task 4.2 contract

**Files likely touched:** web uploader components/hooks/routes and tests

**Estimated scope:** Medium

### Task 4.4: Finalize and enqueue idempotently

**Description:** Durably bridge verified upload completion to one logical processing job.

**Acceptance criteria:**
- [ ] Completion and outbox write are atomic.
- [ ] Redis outage cannot lose committed work.
- [ ] Repeated completion/publishing remains logically idempotent.

**Verification:**
- [ ] Integration tests simulate duplicate calls and Redis failure/recovery.

**Dependencies:** Tasks 4.1, 4.2

**Files likely touched:** upload service, outbox repository/publisher, tests

**Estimated scope:** Medium

### Checkpoint 4

- [ ] Authorized direct multipart upload works and is resumable.
- [ ] Tenant/Event scope and job idempotency are proven.

## Phase 5 — Background indexing

### Task 5.1: Implement reliable Redis worker

**Description:** Consume versioned jobs with bounded concurrency, retries, reclaim, idempotency, and DLQ.

**Acceptance criteria:**
- [ ] Duplicate/crashed/malformed jobs do not corrupt state.
- [ ] Retry/backoff/max-attempt behavior is deterministic.
- [ ] Logs contain correlation IDs but no biometric data or signed URLs.

**Verification:**
- [ ] `uv run --project workers/photo-worker pytest`

**Dependencies:** Task 4.4

**Files likely touched:** `workers/photo-worker/src/photo_worker/jobs.py`, consumer modules, settings, tests

**Estimated scope:** Medium

### Task 5.2: Add safe image derivatives

**Description:** Validate originals and generate metadata-stripped preview/thumbnail variants.

**Acceptance criteria:**
- [ ] Oversized, corrupt, unsafe, and decompression-risk inputs fail safely.
- [ ] Orientation and configured derivative dimensions are correct.
- [ ] Gallery variants never use the original by default.

**Verification:**
- [ ] Python unit/integration tests pass.

**Dependencies:** Task 5.1

**Files likely touched:** worker image module, storage adapter, tests

**Estimated scope:** Medium

### Task 5.3: Expose internal inference endpoint

**Description:** Wrap the approved AI pipeline in an internal-only, bounded, typed endpoint.

**Acceptance criteria:**
- [ ] Request limits/timeouts/authentication and typed errors are enforced.
- [ ] Inputs are not persisted or logged.
- [ ] Caddy cannot route public traffic to the service.

**Verification:**
- [ ] FastAPI tests and Compose network smoke tests pass.

**Dependencies:** Checkpoint 1

**Files likely touched:** Face AI `main.py`, internal schemas/auth, tests, Compose config

**Estimated scope:** Medium

### Task 5.4: Persist and index faces idempotently

**Description:** Join worker, Face AI, PostgreSQL, Qdrant, and MinIO while surviving partial failures.

**Acceptance criteria:**
- [ ] Qdrant payload always includes organization, Event, photo, and face scope.
- [ ] READY is set only after derivatives, face rows, and vector upserts succeed.
- [ ] Retries/compensation handle partial failures without duplicate logical faces.

**Verification:**
- [ ] Multi-face, no-face, duplicate, and partial-failure integration tests pass.

**Dependencies:** Tasks 5.1–5.3

**Files likely touched:** worker orchestrator/adapters, API internal completion path, integration tests

**Estimated scope:** Medium

### Task 5.5: Add processing progress and retry UI

**Description:** Surface trusted aggregate Event progress and authorized failed-photo reprocessing.

**Acceptance criteria:**
- [ ] UI polls and renders queued/processing/ready/failed counts.
- [ ] Safe user-facing failures do not expose internals.
- [ ] Reprocess accepts only eligible failed photos and is idempotent.

**Verification:**
- [ ] API tests, component tests, and progress/retry browser flow pass.

**Dependencies:** Task 5.4

**Files likely touched:** status/reprocess contract and handlers, Event UI, tests

**Estimated scope:** Medium

### Checkpoint 5

- [ ] Upload automatically creates derivatives and indexed faces.
- [ ] Failures retry safely and terminate visibly in FAILED/DLQ.

## Phase 6 — Customer selfie search

### Task 6.1: Define privacy-preserving search contract

**Description:** Specify bounded multipart search, consent, policy, pagination, and typed face-count errors.

**Acceptance criteria:**
- [ ] Contract requires consent and enforces Event eligibility.
- [ ] MVP rejects selfies that contain anything other than exactly one face.
- [ ] Result schema reveals only authorized photo data.

**Verification:**
- [ ] Contract lint and policy tests pass.

**Dependencies:** Checkpoint 5

**Files likely touched:** OpenAPI contract, search domain policy/tests

**Estimated scope:** Small

### Task 6.2: Implement ephemeral Event-scoped search

**Description:** Infer without storing selfie bytes, query only the requested Event, then rank and deduplicate photo results.

**Acceptance criteria:**
- [ ] Qdrant query always includes organization/Event filters and approved threshold.
- [ ] Results deduplicate by photo ID and rank by best similarity.
- [ ] Selfie bytes are absent from storage, logs, and post-request state.

**Verification:**
- [ ] Threshold/ranking/dedup tests and adversarial cross-Event integration tests pass.

**Dependencies:** Task 6.1 and Checkpoint 1 threshold approval

**Files likely touched:** search handlers/service, Face AI client, Qdrant/photo adapters, tests

**Estimated scope:** Medium

### Task 6.3: Build selfie and gallery UI

**Description:** Deliver consent, camera/file input, search feedback, and an accessible responsive result gallery.

**Acceptance criteria:**
- [ ] Browser does not persist the selfie beyond the active flow.
- [ ] UI handles validation, searching, no results, errors, pagination, and preview.
- [ ] QR/link→selfie→results works end-to-end.

**Verification:**
- [ ] Component tests and Playwright customer flow pass.

**Dependencies:** Task 6.2 contract

**Files likely touched:** public Event page, selfie/gallery components, tests

**Estimated scope:** Medium

### Task 6.4: Implement controlled downloads

**Description:** Issue authorized short-lived original downloads and safely audit individual/bounded bulk requests.

**Acceptance criteria:**
- [ ] Server enforces Event download policy and result scope.
- [ ] Signed access expires and cannot be reused outside scope.
- [ ] Successful/denied bulk behavior is rate-limited and auditable.

**Verification:**
- [ ] Authorization, expiry, scope, audit, and rate-limit tests pass.

**Dependencies:** Tasks 6.2, 6.3

**Files likely touched:** download contract/service/handler, gallery controls, tests

**Estimated scope:** Medium

### Checkpoint 6 — MVP functional completion

- [ ] Photographer flow works from registration through QR generation.
- [ ] Customer flow works from QR through authorized gallery/download.
- [ ] Automated tests prove cross-tenant and cross-Event isolation.

## Phase 7 — Hardening and release readiness

### Task 7.1: Add abuse and HTTP controls

**Description:** Add endpoint-specific request/rate limits and browser/API security policy.

**Acceptance criteria:**
- [ ] Body/time/rate limits and safe errors match endpoint sensitivity.
- [ ] CORS, CSRF/cookies, headers, and request IDs are configured deliberately.
- [ ] Upload/search abuse cases are covered by tests.

**Verification:**
- [ ] Security middleware tests and negative browser/API tests pass.

**Dependencies:** Checkpoint 6

**Files likely touched:** API middleware/config, Caddy config, web security config, tests

**Estimated scope:** Medium

### Task 7.2: Add observability and operational health

**Description:** Add privacy-safe logs, metrics, traces, readiness, and runbooks.

**Acceptance criteria:**
- [ ] Signals cover API, queue, inference, upload, and dependencies without high-cardinality biometric/user labels.
- [ ] Liveness and readiness have distinct semantics.
- [ ] Dependency failures emit actionable sanitized signals.

**Verification:**
- [ ] Failure-injection smoke tests and runbook checks pass.

**Dependencies:** Task 7.1

**Files likely touched:** service instrumentation, health endpoints, Compose monitoring, docs

**Estimated scope:** Medium

### Task 7.3: Implement lifecycle deletion and backup restore

**Description:** Make deletion complete and idempotent, then prove recovery procedures.

**Acceptance criteria:**
- [ ] Event/Photo deletion removes or tombstones PostgreSQL, MinIO, Qdrant, and queued work consistently.
- [ ] Selfie/search retention and audit retention are documented and enforced.
- [ ] PostgreSQL/MinIO/Qdrant restore drill succeeds on disposable infrastructure.

**Verification:**
- [ ] Deleted data is neither searchable nor downloadable; documented restore test passes.

**Dependencies:** Task 7.2

**Files likely touched:** deletion workflows/adapters, backup scripts/config, operations docs, tests

**Estimated scope:** Medium

### Task 7.4: Run release verification and complete documentation

**Description:** Validate a fresh deployment and document operation, limitations, licenses, and benchmark evidence.

**Acceptance criteria:**
- [ ] Fresh migration and full photographer/customer E2E pass.
- [ ] Security, privacy, license, backup, and approved benchmark checklists are complete.
- [ ] README, architecture, API/deployment/runbooks, and `.env.example` match reality.

**Verification:**
- [ ] `make check`
- [ ] `make test`
- [ ] `make build`
- [ ] `docker compose config`
- [ ] Web E2E and one authorized sample Event pass.

**Dependencies:** Tasks 7.1–7.3

**Files likely touched:** project documentation, environment examples, focused fixes discovered by verification

**Estimated scope:** Medium

### Final checkpoint

- [ ] MVP passes from a fresh deployment.
- [ ] AI remains within the approved benchmark gate.
- [ ] Operational and compliance responsibilities are documented and tested.

## Deferred backlog (V2–V4)

- V2: larger/resumable batch tuning, worker/GPU scaling, password Events, albums/share, analytics, duplicate detection, and face quality.
- V3: subscriptions, quotas, teams, payments/invoices, branding, custom domains, watermarking, and photo sales.
- V4: API/AI replicas, clustered Redis/PostgreSQL/Qdrant/object storage, and CDN.

## Parallelization constraints

Contracts must be accepted before API/UI work separates. Do not parallelize edits to migrations, OpenAPI, generated clients, or shared state machines without one owner. Worker reliability and derivatives may proceed independently before their integration; frontend may proceed against generated contracts and deterministic mocks.
