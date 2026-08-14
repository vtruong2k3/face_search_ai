# Face AI service

The current model-backed implementation is a local, personal, non-commercial proof of concept using InsightFace `buffalo_l` on CPU. The pretrained weights are not approved for commercial SaaS use or redistribution.

## Local model setup

Keep weights outside this repository. Arrange the externally obtained model pack as:

```text
/absolute/external/root/
└── models/
    └── buffalo_l/
        ├── det_10g.onnx
        └── w600k_r50.onnx
```

Configure:

```bash
export FACE_AI_INSIGHTFACE_ENABLED=true
export FACE_AI_INSIGHTFACE_MODEL_ROOT=/absolute/external/root
export FACE_AI_INSIGHTFACE_PACK=buffalo_l
export FACE_AI_ONNX_PROVIDER=CPUExecutionProvider
```

The adapter requires the canonical SCRFD `det_10g.onnx` and ArcFace `w600k_r50.onnx` artifacts before creating `FaceAnalysis`, so normal startup and tests do not implicitly download model weights. InsightFace is disabled by default. When enabled, readiness initializes one cached pipeline per effective model configuration and returns `503 not_ready` if the external root, CPU provider, pack, or recognition model is unavailable. Health output uses sanitized states and never returns the model root or raw initialization error.

Install dependencies from the locked project environment. When `uv` is available:

```bash
uv sync --project services/face-ai
```

## Pipeline assumptions

- InsightFace `buffalo_l`
- SCRFD detection at 640×640, threshold configurable and defaulting to 0.5
- Bounding box plus five landmarks
- Five-point alignment to 112×112
- ArcFace 512D embedding
- CPU ONNX Runtime
- Final deterministic L2 normalization in `FacePipeline`

Validated images use RGB HWC `uint8`. The InsightFace adapter converts RGB to BGR only at model boundaries.

## Verification

Unit tests use injected fakes and never require models or network access:

```bash
services/face-ai/.venv/bin/pytest services/face-ai/tests
services/face-ai/.venv/bin/ruff check services/face-ai/src services/face-ai/tests
services/face-ai/.venv/bin/mypy services/face-ai/src
```

Before a real benchmark, record the SHA-256 of every local ONNX artifact and use only an authorized image dataset. A real CPU smoke run remains pending until both the external model pack and an authorized image are configured.

## Reproducible benchmark core

`config/benchmark.example.json` is the executable strict-JSON manifest example. It records frozen dataset/model identifiers, model and per-image SHA-256 checksums, relative enrollment/query paths, search limits, and threshold candidates. A dataset version is only a label; the per-entry checksums cryptographically bind the manifest fingerprint to the listed bytes. The validator rejects unknown fields, duplicate IDs or paths, absolute/traversing paths, symlink escapes, malformed or mismatched checksums, and use of `approved_non_commercial_poc` outside `personal_non_commercial_poc` mode.

Calculate each image checksum locally without copying the dataset into Git, for example:

```bash
sha256sum /external/authorized-dataset/enrollment/image-001.jpg
```

The model-independent benchmark modules provide deterministic subject-deduplicated observations, Top-K accuracy, precision, recall, FAR, FRR, no-face/ambiguous rates, and linear-interpolated latency percentiles. Optional query-only condition labels are strict opaque categorical codes declared in the manifest and fingerprint-bound; they must never encode names, locations, or free-form identifying information. Every declared slice is emitted deterministically, but slices below the manifest's fixed minimum size are marked `suppressed` and expose no outcome metrics. Available slices reuse global thresholds and metric formulas as diagnostic evidence only; they do not select separate thresholds. Performance output aggregates decode/validation, detection, per-image alignment and embedding totals, conditional vector search, and end-to-end query timing, plus serial queries per second. Acceptance uses `score >= threshold`. Reports are canonical aggregate JSON under an ignored `benchmark-results/` or `benchmark-output/` directory and do not include paths, image bytes, embeddings, raw timing samples, or per-query identity observations.

Real-run reports also include a strict sanitized runtime descriptor: OS family, architecture, logical CPU count, Python and relevant library versions, configured ONNX provider, InsightFace pack and detection settings, and the serial execution policy. Collection reads configuration and package/platform metadata only; it does not initialize models, access dataset files, or contact Qdrant. It intentionally excludes hostnames, usernames, paths, URLs, environment dumps, and hardware serial identifiers. Logical CPU count is neither physical-core count nor CPU utilization, and this descriptor is reproducibility context—not real performance evidence.

After `uv sync --locked --project services/face-ai`, validate a frozen manifest without initializing models or Qdrant:

```bash
uv run --locked --project services/face-ai face-ai-benchmark validate \
  --manifest /external/benchmark.json \
  --dataset-root /external/authorized-dataset
```

The validation command resolves every explicit entry under the external root and streams its SHA-256 before any model or Qdrant initialization. It does not retain dataset bytes. Run it on a controlled machine where the external dataset cannot be modified concurrently; preflight binds the run to the bytes observed at verification time.

Run the deterministic no-biometric-data harness end to end:

```bash
uv run --locked --project services/face-ai face-ai-benchmark synthetic \
  --output benchmark-results/synthetic.json
```

The synthetic command self-checks fixed expected metrics and writes byte-stable aggregate output, including deterministic synthetic stage timings, serial throughput, and process-resource evidence. These values verify the instrumentation and aggregation contract only; they do not demonstrate real model quality, CPU performance, or memory use. The fixed `enrollment_primed_serial` policy processes every enrollment entry exactly once in manifest-relative order before every query, treats enrollment inference as the warm-up phase, and includes the first query in all query metrics. Enrollment timing is reported separately: end-to-end includes exact byte loading plus pipeline processing, while stage percentiles use per-image pipeline timings and distinguish all attempts from indexed vectors. Vector-index setup/validation, explicit 100-record batched blocking upsert, and teardown are separate scalar durations. Synthetic lifecycle values are not real Qdrant evidence. Process resources report user-plus-system CPU time consumed inside the complete runner boundary and the post-run process-lifetime peak RSS high-water mark. Linux RSS KiB is normalized to bytes; macOS RSS is already bytes. Peak RSS may include model initialization and earlier process work, so it is not runner-only allocation or a memory delta. Unsupported probes produce sanitized unavailable values. CPU time is not CPU utilization, physical-core usage, concurrency, or service capacity. Cold process startup, model/session initialization, and first-ever inference are explicitly not measured. Serial query throughput excludes model initialization, enrollment/upsert, collection lifecycle, teardown, and concurrent load capacity.

After the external model pack, exact artifact checksums, authorized frozen dataset, and Qdrant are configured, execute the composed real benchmark path with an explicit calibration policy:

```bash
uv run --locked --project services/face-ai face-ai-benchmark run \
  --manifest /external/benchmark.json \
  --dataset-root /external/authorized-dataset \
  --output benchmark-results/real.json \
  --max-far 0.01 \
  --min-recall 0.90 \
  --max-frr 0.10
```

The command reads `FACE_AI_INSIGHTFACE_*`, `FACE_AI_ONNX_PROVIDER`, and `FACE_AI_QDRANT_URL` from the environment. It first streams and verifies every manifest image against its entry checksum. Only after the complete dataset preflight succeeds does it verify the canonical local SCRFD and ArcFace artifact hashes, initialize model sessions, and construct the Qdrant index. It then creates a temporary deterministic cosine collection and always attempts teardown after runner startup. Errors are sanitized. Unit tests prove composition only; they are not real accuracy, threshold, latency, or throughput evidence.

The current runner is dependency-injected and unit-tested with synthetic inputs. A real report and threshold recommendation remain pending the authorized frozen dataset, exact local model checksums, successful CPU smoke run, and reachable benchmark Qdrant. Phase 2 must not start until that real report is reviewed and explicitly approved.
