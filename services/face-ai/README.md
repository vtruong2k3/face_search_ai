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

`config/benchmark.example.json` is the executable strict-JSON manifest example. It records frozen dataset/model identifiers, SHA-256 checksums, relative enrollment/query paths, search limits, and threshold candidates. The validator rejects unknown fields, duplicate IDs or paths, absolute/traversing paths, symlink escapes, malformed checksums, and use of `approved_non_commercial_poc` outside `personal_non_commercial_poc` mode.

The model-independent benchmark modules provide deterministic subject-deduplicated observations, Top-K accuracy, precision, recall, FAR, FRR, no-face/ambiguous rates, linear-interpolated latency percentiles, and an offline threshold sweep. Acceptance uses `score >= threshold`. Reports are canonical aggregate JSON under an ignored `benchmark-results/` or `benchmark-output/` directory and do not include paths, image bytes, embeddings, or per-query identity observations.

After `uv sync --locked --project services/face-ai`, validate a frozen manifest without initializing models or Qdrant:

```bash
uv run --locked --project services/face-ai face-ai-benchmark validate \
  --manifest /external/benchmark.json \
  --dataset-root /external/authorized-dataset
```

Run the deterministic no-biometric-data harness end to end:

```bash
uv run --locked --project services/face-ai face-ai-benchmark synthetic \
  --output benchmark-results/synthetic.json
```

The synthetic command self-checks fixed expected metrics and writes byte-stable aggregate output. It does not demonstrate real model quality or CPU performance.

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

The command reads `FACE_AI_INSIGHTFACE_*`, `FACE_AI_ONNX_PROVIDER`, and `FACE_AI_QDRANT_URL` from the environment. Before initializing model sessions, creating Qdrant collections, or reading dataset images, it hashes the canonical local SCRFD and ArcFace artifacts and requires an exact match with the manifest. It then creates a temporary deterministic cosine collection and always attempts teardown after runner startup. Errors are sanitized. Unit tests prove composition only; they are not real accuracy, threshold, latency, or throughput evidence.

The current runner is dependency-injected and unit-tested with synthetic inputs. A real report and threshold recommendation remain pending the authorized frozen dataset, exact local model checksums, successful CPU smoke run, and reachable benchmark Qdrant. Phase 2 must not start until that real report is reviewed and explicitly approved.
