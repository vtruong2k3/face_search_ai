# Face Search AI — Task Status

Last updated: 2026-08-14

## Phase 0 — Governance

- [x] Task 0.1: Materialize `tasks/plan.md` and `tasks/todo.md`
- [x] Task 0.2: Establish AI data and license controls
- [x] Checkpoint 0: Governance and sensitive-artifact scan

## Phase 1 — AI feasibility PoC

- [x] Task 1.1: Define stable AI domain interfaces
- [x] Task 1.2: Add model lifecycle and ONNX provider selection
- [x] Task 1.3: Implement detection, alignment, and embedding (personal non-commercial buffalo_l PoC; cached runtime/readiness integration unit-verified; real CPU smoke pending local pack/image)
- [x] Task 1.4: Add benchmark Qdrant adapter (unit-verified; Compose integration pending reachable Qdrant)
- [ ] Task 1.5: Build reproducible benchmark runner
  - [x] Strict JSON manifest validation and stable fingerprints
  - [x] Dependency-injected enrollment/query runner
  - [x] Privacy-safe deterministic aggregate report writer
  - [x] Synthetic unit verification with mathematically known results
  - [x] Executable offline validation and synthetic benchmark CLI
  - [x] Executable real-benchmark composition path unit-verified
  - [x] Locked environment includes and imports InsightFace
  - [ ] Real authorized dataset and Qdrant benchmark run
- [ ] Task 1.6: Calibrate threshold and report decision
  - [x] Deterministic threshold sweep and policy implementation
  - [x] Synthetic calibration unit verification
  - [ ] Real threshold recommendation from frozen authorized observations
  - [ ] Record measured CPU latency and throughput
- [ ] Checkpoint 1: User approves AI quality gate

## Phase 2 — MVP platform foundation

- [ ] Task 2.1: Harden infrastructure and add migrations
- [ ] Task 2.2: Add Go persistence boundaries
- [ ] Task 2.3: Implement authentication slice
- [ ] Task 2.4: Implement tenant authorization foundation
- [ ] Checkpoint 2: Auth and tenant isolation

## Phase 3 — Event management

- [ ] Task 3.1: Implement Event contract and domain
- [ ] Task 3.2: Build photographer Event UI
- [ ] Task 3.3: Add public Event and QR
- [ ] Checkpoint 3: Private management and public access policy

## Phase 4 — Secure photo ingestion

- [ ] Task 4.1: Define Photo lifecycle and object policy
- [ ] Task 4.2: Implement signed multipart upload
- [ ] Task 4.3: Build resumable uploader UI
- [ ] Task 4.4: Finalize and enqueue idempotently
- [ ] Checkpoint 4: Direct upload and durable job handoff

## Phase 5 — Background indexing

- [ ] Task 5.1: Implement reliable Redis worker
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

## Mandatory gate

Phase 2 must not begin until the user reviews the frozen Phase 1 benchmark and explicitly approves the model, threshold policy, accuracy metrics, and CPU performance. The current buffalo_l approval is restricted to a personal non-commercial PoC and does not authorize commercial SaaS release.

## Deferred V2–V4

Payment, teams, clustering, branding, custom domains, advanced duplicate detection, GPU pools, and distributed scaling are out of the current implementation scope.
