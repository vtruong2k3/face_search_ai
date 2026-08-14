# Face Search AI — Agent Instructions

These instructions apply to the entire repository. More specific `AGENTS.md` files may add rules for their own subtree, but they must not weaken or contradict this file.

## Mission

Build **Face Search AI** as a production-oriented, multi-tenant SaaS for ingesting event photos, detecting and indexing faces, and letting authorized users find matching photos.

The repository is under active development. Do not mistake placeholders, scaffolding, or planned infrastructure for completed functionality. Verify the current implementation before making claims about project status.

## Repository Boundaries

Work only in this repository unless the user explicitly asks otherwise:

- Target repository: `face-search-ai/`
- Read-only reference: `../face-recognition/`
- Read-only reference: `../Face-Search/`
- Read-only reference: `../immich/`

The three neighboring projects are references, not implementation workspaces. Never edit, format, generate files in, install dependencies in, or commit changes to them.

## Reference and Licensing Policy

- Study reference projects to understand workflows, architecture, data models, UX ideas, edge cases, and performance techniques.
- Reimplement required behavior independently in the conventions and architecture of this repository.
- Do **not** copy or closely translate source code, tests, assets, UI, text, schemas, or configuration from reference projects.
- `Face-Search` is GPL and `immich` is AGPL; treat both as architectural references only.
- Treat models, weights, datasets, fonts, icons, and other artifacts as separately licensed. Do not add them without confirming that their licenses permit the intended commercial use.
- Record relevant external inspiration and license decisions in project documentation when they materially affect implementation.
- If licensing is unclear, stop and ask the user rather than copying the material.

## Architecture

Preserve these ownership boundaries:

- `apps/web`: Next.js/TypeScript UI, rendering, and browser interaction.
- `apps/api`: Go public API, authentication, authorization, tenancy, domain logic, persistence orchestration, and public contracts.
- `services/face-ai`: Python internal service for model loading and synchronous inference only.
- `workers/photo-worker`: Python asynchronous photo-processing jobs.
- `packages/contracts/openapi.yaml`: source of truth for the public HTTP API.
- `infra`: deployment and local-infrastructure configuration.
- `docs`: architecture, development decisions, and operational documentation.

Dependency rules:

- Browser clients must never call `face-ai`, workers, databases, object storage, Redis, or Qdrant directly.
- Public business APIs live under `/api/v1`; health checks may remain unversioned.
- Internal services must not be exposed through the public reverse proxy without an explicit architecture decision.
- Do not move business authorization or tenant enforcement into the frontend or AI service.
- Keep model-specific implementation inside `services/face-ai`.
- Keep asynchronous processing inside `workers/photo-worker`.
- Avoid introducing cross-service shared code that couples independent runtimes. Share contracts, not internal implementations.

Read `docs/architecture.md` and `docs/development.md` before making architectural or cross-service changes. Update those documents when an accepted change makes them inaccurate.

## Contract-First Development

For public API work:

1. Design or update `packages/contracts/openapi.yaml` first.
2. Validate the OpenAPI contract.
3. Implement domain behavior in the Go API.
4. Implement the HTTP transport separately from domain logic.
5. Regenerate web API types when the contract changes; do not hand-edit generated files.
6. Add tests at contract and service boundaries.
7. Connect the frontend only after the contract and behavior are stable.

Do not invent undocumented request or response shapes independently in web and API code.

## Security and Data Rules

Face embeddings and biometric-derived data are sensitive data.

- Enforce authentication, authorization, tenant isolation, and event/resource ownership server-side.
- Apply tenant and authorization filters to every read, write, search, export, and background job.
- Use server-generated identifiers where trust boundaries require them.
- Never log raw credentials, access tokens, signed URLs, face embeddings, or unnecessary personal data.
- Never commit secrets. Configuration must come from environment variables, with safe placeholders in `.env.example`.
- Validate file type, size, dimensions, and decoding before image processing. Do not trust file names or client MIME types.
- Treat image metadata and uploaded content as untrusted input.
- Use bounded concurrency, timeouts, retries with backoff, idempotency, and dead-letter/error states for background work.
- Do not expose storage objects publicly by default. Prefer short-lived, scope-limited signed access.
- Do not silently weaken security, privacy, retention, or tenant boundaries to simplify implementation.

## Engineering Workflow

Before coding:

1. Read the nearest `AGENTS.md` and relevant documentation.
2. Inspect existing code and tests; do not assume a feature is absent or complete.
3. Identify the owning service and boundary.
4. For non-trivial work, state or maintain a short implementation plan before editing.
5. Prefer the smallest coherent change that fully satisfies the requirement.

While coding:

- Match surrounding naming, formatting, error handling, and comment density.
- Keep modules focused and dependencies explicit.
- Prefer boring, maintainable solutions over speculative abstractions.
- Do not add dependencies when the platform or an existing dependency already solves the problem adequately.
- Do not change generated files manually.
- Do not perform unrelated refactors in feature or bug-fix changes.
- Preserve backward compatibility unless a breaking change is explicitly approved and documented.
- Add or update tests with behavior changes.

After coding:

1. Run the narrowest relevant tests first.
2. Run formatting, linting, type checking, and broader tests for every affected service.
3. Review the diff for secrets, generated artifacts, debug code, accidental reference-project changes, and unrelated edits.
4. Report exactly what was changed, what was verified, and what remains unverified.

Never claim that a feature or the codebase is complete solely because files exist or a build passes. Check acceptance behavior and tests.

## Commands and Verification

Repository-level checks:

```bash
make check
make test
make build
docker compose config
```

Web (`apps/web`):

```bash
npm run lint
npm run typecheck
npm run test
npm run build
npm run test:e2e
npm run contracts:lint
```

Go API (`apps/api`):

```bash
go fmt ./...
go vet ./...
go test ./...
go build ./cmd/api
```

Python services:

```bash
uv run --project services/face-ai pytest
uv run --project workers/photo-worker pytest
```

Run only commands relevant to the change unless the user requests a full verification pass. If a command cannot run because infrastructure, credentials, models, browsers, or dependencies are unavailable, state that clearly; do not report it as passing.

## Web-Specific Rule

`apps/web/AGENTS.md` contains a generated Next.js rule block. Preserve it. Before changing Next.js APIs, conventions, configuration, or file structure, read the relevant documentation shipped in `apps/web/node_modules/next/dist/docs/` and follow deprecation notices rather than relying on model memory.

## Definition of Done

A change is done only when:

- Behavior matches the agreed requirement.
- Service and trust boundaries remain intact.
- Public contract and generated consumers are synchronized when applicable.
- Errors and edge cases are handled deliberately.
- Relevant tests are added or updated and pass.
- Relevant lint, type, formatting, and build checks pass.
- Documentation and `.env.example` are updated when behavior or configuration changes.
- No secrets, large generated outputs, model weights, user data, or accidental artifacts are introduced.
- Remaining limitations and skipped verification are explicitly reported.

## Decision Priorities

When rules or goals appear to conflict, use this order:

1. User safety, privacy, security, and legal/license compliance.
2. Explicit user requirements and approved architecture decisions.
3. Tenant isolation and data correctness.
4. Public contract compatibility.
5. Reliability and observability.
6. Performance supported by evidence.
7. Developer convenience.

Ask the user when a decision materially changes product behavior, security/privacy guarantees, data retention, licensing posture, public contracts, or system architecture.
