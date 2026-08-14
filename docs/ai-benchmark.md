# AI Benchmark Data and License Gate

Face photos, selfies, embeddings, and biometric-derived reports are sensitive data. The AI proof-of-concept must not process a dataset or model until the checklist in this document is complete. This gate applies even when files remain on a developer machine.

## Dataset contract

The benchmark dataset is supplied by the user through an external local directory. It must never be copied into this repository, a container image, logs, test fixtures, or CI artifacts.

Before use, record and approve:

- The dataset owner or custodian.
- The lawful basis and documented consent for face-recognition benchmarking.
- The allowed purpose, operators, environment, and retention period.
- Whether commercial product evaluation is permitted.
- The deletion date and person responsible for deletion.
- Any restrictions on reporting or derived biometric data.

Use only opaque subject and image IDs in manifests and reports. Do not include names, email addresses, phone numbers, event names, or other direct identifiers.

## Required split

The manifest defines immutable roles:

- `enrollment`: event photos that are indexed.
- `query`: authorized selfies used to evaluate retrieval.

A file may not appear in both roles. The benchmark runner must reject duplicate relative paths, duplicate image IDs, and a subject/image split that violates the frozen evaluation protocol. Changes to the split create a new dataset version and invalidate direct comparison with earlier reports.

## Local-only paths

Set the dataset root at runtime through configuration or an environment variable. Configuration committed to Git may contain only a placeholder such as `/absolute/path/outside/repository`.

All manifest image paths must be relative to that external root. Reject absolute paths and path traversal. The runner must not discover or scan unrelated directories automatically.

## Forbidden tracked artifacts

Never commit or publish:

- Original photos, thumbnails, crops, or selfies.
- Face embeddings or vector database snapshots.
- Model weights or runtime engine files.
- Reports containing direct identities or images.
- Access credentials, signed URLs, storage dumps, or debug payloads.

Benchmark output belongs in an ignored `benchmark-results/` directory. Reports intended for review must use aggregate metrics and opaque IDs. False-positive and false-negative examples must not embed images.

## Model artifact review

Every detector and embedding model requires an independent review. Record:

- Model name and exact version.
- Upstream source URL and publisher.
- Artifact SHA-256 checksum.
- Code license and weight license separately.
- Dataset or use restrictions disclosed by the publisher.
- Explicit decision on commercial use, with reviewer and review date.

A compatible code license does not imply that pretrained weights or their training data are commercially usable. Do not download, mount, or execute a model with status other than `approved` for commercial work. A model marked `approved_non_commercial_poc` may be used only for the explicitly recorded local research scope and does not satisfy the SaaS release gate.

### Current personal PoC model decision

For the local personal proof of concept, the project owner approved InsightFace `buffalo_l` on 2026-08-14 with status `approved_non_commercial_poc`. The pack uses SCRFD detection and ArcFace recognition through ONNX Runtime on CPU. Its pretrained weights are treated as non-commercial-research-only: commercial use, hosted SaaS deployment, and redistribution are not approved.

The implementation expects the pack at an external root under `models/buffalo_l` and refuses to initialize when it is absent, preventing implicit downloads. Model files remain outside this repository. Exact local ONNX artifact SHA-256 values must be recorded before a real benchmark run. Processing assumptions are RGB input validation, RGB-to-BGR conversion at the InsightFace boundary, 640×640 detection, five-point 112×112 alignment, a 512D ArcFace vector, and final L2 normalization by the model-neutral pipeline.

## Runtime and reporting controls

- Run the PoC on a controlled machine with least-privilege file access.
- Do not log image bytes, embeddings, local paths, names, or signed URLs.
- Keep temporary selfie/image bytes in memory only where practical and delete temporary files immediately.
- Keep benchmark collections separate and filter every vector search by the configured dataset/Event partition.
- Record seed, manifest version, model checksum, preprocessing version, threshold sweep, and hardware class for reproducibility.
- Delete dataset access, temporary files, benchmark vectors, and sensitive outputs at the approved retention deadline.

## Frozen metric protocol

The executable manifest format is strict JSON; see `services/face-ai/config/benchmark.example.json`. One query is either a known-subject match query or an impostor query with a null expected subject. Rankings are deduplicated by opaque subject ID, retaining the highest score with deterministic tie ordering. A score is accepted when `score >= threshold`.

- Top-K is measured over known-subject queries and is independent of the acceptance threshold.
- TP is an accepted top-1 expected subject.
- FP is an accepted impostor result or an accepted wrong subject for a known query.
- FN is a known query without an accepted correct top-1 result.
- TN is an impostor query without an accepted result.
- Undefined zero-denominator rates are reported as null, not zero.
- No-face and ambiguous multi-face query rates use all query entries.
- p50/p90/p95/p99 use linear interpolation over sorted non-negative latency samples.

Threshold calibration operates offline over one frozen result set. A recommendation must satisfy explicit maximum FAR, optional maximum FRR, and minimum recall limits; otherwise the report states `no_feasible_threshold`. Reports contain aggregates and reproducibility fingerprints only by default. Real metric values are not available until the external authorized dataset and model artifacts are configured.

The offline CLI can validate all manifest paths without initializing InsightFace or Qdrant:

```bash
uv run --locked --project services/face-ai face-ai-benchmark validate \
  --manifest /external/benchmark.json \
  --dataset-root /external/authorized-dataset
```

`face-ai-benchmark synthetic --output benchmark-results/synthetic.json` exercises fixed observations, metrics, calibration, and canonical reporting without biometric data. Passing this command verifies the harness only; it is not evidence for real model accuracy, threshold suitability, or CPU performance.

## Approval checklist

Dataset:

- [ ] Custodian and lawful basis are recorded.
- [ ] Consent permits face-recognition benchmarking for this purpose.
- [ ] Commercial evaluation and reporting restrictions are understood.
- [ ] Enrollment/query split and dataset version are frozen.
- [ ] Retention deadline and deletion owner are recorded.

Model:

- [ ] Source and publisher are recorded.
- [ ] Artifact checksum has been verified.
- [ ] Code, weights, and disclosed dataset restrictions were reviewed separately.
- [ ] Commercial-use decision is `approved`.
- [ ] Reviewer and review date are recorded.

Execution:

- [ ] Dataset root is outside the repository.
- [ ] Manifest uses opaque IDs and relative paths only.
- [ ] Sensitive outputs are ignored by Git and excluded from logs.
- [ ] Benchmark collection and cleanup procedure are configured.

The AI benchmark may start only when every item above is complete.
