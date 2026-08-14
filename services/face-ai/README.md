# Face AI service

The current model-backed implementation is a local, personal, non-commercial proof of concept using InsightFace `buffalo_l` on CPU. The pretrained weights are not approved for commercial SaaS use or redistribution.

## Local model setup

Keep weights outside this repository. Arrange the externally obtained model pack as:

```text
/absolute/external/root/
└── models/
    └── buffalo_l/
        └── *.onnx
```

Configure:

```bash
export FACE_AI_INSIGHTFACE_ENABLED=true
export FACE_AI_INSIGHTFACE_MODEL_ROOT=/absolute/external/root
export FACE_AI_INSIGHTFACE_PACK=buffalo_l
export FACE_AI_ONNX_PROVIDER=CPUExecutionProvider
```

The adapter verifies that `models/buffalo_l` exists before creating `FaceAnalysis`, so normal startup and tests do not implicitly download model weights.

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
