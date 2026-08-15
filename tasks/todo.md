# Face Search AI — Task Status

Last updated: 2026-08-15

## Phase 0 — Governance

- [x] Task 0.1: Materialize `tasks/plan.md` and `tasks/todo.md`
- [x] Task 0.2: Establish AI data and license controls
- [x] Checkpoint 0: Governance and sensitive-artifact scan

## Phase 1 — AI feasibility PoC

- [x] Task 1.1: Define stable AI domain interfaces
- [x] Task 1.2: Add model lifecycle and ONNX provider selection
- [x] Task 1.3: Implement detection, alignment, and embedding (personal non-commercial buffalo_l PoC; cached runtime/readiness unit-verified; real CPU model initialization verified; authorized-image inference smoke pending)
- [x] Task 1.4: Add benchmark Qdrant adapter (unit and real local Qdrant integration verified)
- [ ] Task 1.5: Build reproducible benchmark runner
  - [x] Strict JSON manifest validation and stable fingerprints
  - [x] Dependency-injected enrollment/query runner
  - [x] Privacy-safe deterministic aggregate report writer
  - [x] Synthetic unit verification with mathematically known results
  - [x] Executable offline validation and synthetic benchmark CLI
  - [x] Executable real-benchmark composition path unit-verified
  - [x] Manifest checksums bound fail-closed to canonical local buffalo_l artifacts
  - [x] Per-entry dataset bytes frozen by SHA-256 with fail-before-side-effects preflight
  - [x] Repeated composed runs produce identical logical and canonical aggregate reports
  - [x] Explicit enrollment-primed warm execution policy with cold-start exclusion
  - [x] Separate aggregate enrollment and vector-index lifecycle timing
  - [x] Locked environment includes and imports InsightFace
  - [ ] Real authorized dataset and Qdrant benchmark run
- [ ] Task 1.6: Calibrate threshold and report decision
  - [x] Deterministic threshold sweep and policy implementation
  - [x] Synthetic calibration unit verification
  - [x] Deterministic stage latency and serial-throughput instrumentation
  - [x] Sanitized hardware/runtime reproducibility metadata
  - [x] Process CPU-time and process-lifetime peak-RSS instrumentation
  - [x] Privacy-safe aggregate condition slicing with sparse suppression
  - [ ] Real threshold recommendation from frozen authorized observations
  - [ ] Record measured CPU latency and throughput
- [ ] Checkpoint 1: Real AI benchmark and release-use approval

## Phase 2 — MVP platform foundation

Phase 2 platform-foundation implementation is authorized to proceed independently of Checkpoint 1. This authorization does not approve a production threshold, real AI quality or CPU-performance claims, commercial use of the current PoC weights, or commercial/release readiness.

- [x] Task 2.1: Harden infrastructure and add migrations (versioned reversible MVP schema; database-enforced tenant integrity/state/idempotency; Compose migration and MinIO initialization gates; disposable up/down/up verification)
- [x] Task 2.2: Add Go persistence boundaries (bounded pgx pool; DBTX and transaction ports; rollback guarantees; sanitized error mapping; exact migration-version readiness; disposable PostgreSQL integration verification)
- [x] Task 2.3: Implement authentication slice (Argon2id registration/login; short-lived in-memory access tokens; hashed opaque refresh-token rotation/replay rejection; scoped HttpOnly cookies; current-user/logout API and minimal Next.js UI; disposable PostgreSQL, HTTP security, and web tests)
- [x] Task 2.4: Implement tenant authorization foundation (central owner/admin/editor/viewer permission matrix; database-fresh active membership checks; trusted request/tenant principals; non-enumerating organization endpoints; bounded request IDs and safe authorization audit records; two-tenant isolation tests; memory-only web organization context)
- [x] Checkpoint 2: Auth and tenant isolation

## Phase 3 — Event management

- [x] Task 3.1: Implement Event contract and domain (tenant-scoped Event CRUD foundation, archive semantics, generated contract types, and trusted processing counters)
- [x] Task 3.2: Build photographer Event UI (tenant-scoped list/create/detail/settings routes; viewer read-only controls; loading, empty, validation, and safe-error states; Vitest coverage)
- [x] Task 3.3: Add public Event and QR (opaque server-generated tokens; uniform private/expired/archived rejection; limited public DTO; canonical public URL and QR tests)
- [x] Checkpoint 3: Private management and public access policy

## Phase 4 — Secure photo ingestion

- [x] Task 4.1: Define Photo lifecycle and object policy (fail-closed lifecycle transitions, server-owned opaque object keys, tenant/Event/Photo-scoped CRUD policy, soft-delete/reprocess controls, and generated contract types)
- [x] Task 4.2: Implement signed multipart upload (bounded upload policy, tenant/Event/Photo-scoped retry-safe sessions, exact-part short-lived MinIO URLs, direct multipart completion verification, idempotent abort, reversible migration, and real disposable MinIO proof)
- [x] Task 4.3: Build resumable uploader UI (tenant/Event-scoped Uppy multipart callbacks, direct-to-MinIO part transfer, bounded retries/concurrency, pause/resume/cancel controls, permission gating, and queue teardown on scope changes)
- [x] Task 4.4: Finalize and enqueue idempotently
- [ ] Checkpoint 4: Direct upload and durable job handoff

## Phase 5 — Background indexing

- [x] Task 5.1: Implement reliable Redis worker
- [ ] Task 5.2: Add safe image derivatives
- [ ] Task 5.3: Expose internal inference endpoint
- [ ] Task 5.4: Persist and index faces idempotently
- [ ] Task 5.5: Add processing progress and retry UI
- [ ] Checkpoint 5: Automatic reliable face indexing

## Phase 6 — Customer selfie search

- [ ] Task 6.1: Define privacy-preserving search contract
- [ ] Task 6.2: Implement ephemeral Event-scoped search
- [ ] Task 6.3: Build selfie and gallery UI
- [ ] Task 6.4: Implement controlled downloads
- [ ] Checkpoint 6: Complete photographer and customer MVP flows

## Phase 7 — Hardening

- [ ] Task 7.1: Add abuse and HTTP controls
- [ ] Task 7.2: Add observability and operational health
- [ ] Task 7.3: Implement lifecycle deletion and backup restore
- [ ] Task 7.4: Run release verification and complete documentation
- [ ] Final checkpoint: Release-ready MVP

## AI evidence and release gate

Phase 2 platform-foundation implementation may proceed while the real Phase 1 benchmark and Checkpoint 1 remain incomplete. The incomplete benchmark items must remain unchecked and must not be represented as real accuracy, threshold, latency, throughput, capacity, or memory evidence. Checkpoint 1 remains required before approving a production face-search threshold, enabling a production model-backed path, or representing the AI path as validated for release. The current `buffalo_l` approval is restricted to a personal non-commercial PoC and does not authorize commercial SaaS use, hosted deployment, redistribution, or commercial release.

## Deferred V2–V4

Payment, teams, clustering, branding, custom domains, advanced duplicate detection, GPU pools, and distributed scaling are out of the current implementation scope.
