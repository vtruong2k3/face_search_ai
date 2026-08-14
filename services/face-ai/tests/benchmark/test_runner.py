from __future__ import annotations

import numpy as np
from face_ai.benchmark.manifest import BenchmarkManifest
from face_ai.benchmark.runner import BenchmarkRunner
from face_ai.domain import (
    BoundingBox,
    DetectedFace,
    EmbeddedFace,
    FacialLandmarks,
    SearchResult,
)
from face_ai.pipeline import PipelineResult, PipelineTimings
from face_ai.validation import ValidatedImage

_FACE = DetectedFace(BoundingBox(0, 0, 2, 2), FacialLandmarks((0, 0), (1, 0), (0.5, 0.5), (0, 1), (1, 1)), 1.0)


class FakePipeline:
    def process(self, content: bytes) -> PipelineResult:
        count = int(content.decode())
        image = ValidatedImage("image/png", 2, 2, np.zeros((2, 2, 3), dtype=np.uint8))
        faces = tuple(EmbeddedFace(_FACE, np.array([1.0, 0.0], dtype=np.float32)) for _ in range(count))
        return PipelineResult(image, faces, PipelineTimings(1.0, 2.0, 3.0, 4.0))


class FakeIndex:
    def __init__(self) -> None:
        self.records = []
        self.searches = []
        self.torn_down = False

    def create(self) -> None: pass
    def upsert(self, records: object, *, batch_size: int = 100) -> None: self.records.extend(records)
    def search(self, embedding: np.ndarray, **kwargs: object) -> list[SearchResult]:
        self.searches.append(kwargs)
        return [SearchResult("img-001:0", "img-001", 0.9)]
    def teardown(self) -> None: self.torn_down = True


def manifest() -> BenchmarkManifest:
    return BenchmarkManifest.from_dict({
        "benchmark_id": "poc-v1", "mode": "personal_non_commercial_poc", "seed": 1,
        "dataset": {"id": "dataset-v1", "version": "v1", "event_id": "event-v1"},
        "model": {"id": "model-v1", "approval": "approved_non_commercial_poc", "detector_sha256": "a" * 64, "embedder_sha256": "b" * 64},
        "search": {"limit": 10, "thresholds": [0.5], "top_k": [1]},
        "entries": [
            {"image_id": "img-001", "subject_id": "subject-1", "path": "enroll/a.png", "role": "enrollment"},
            {"image_id": "img-002", "subject_id": "subject-1", "path": "query/b.png", "role": "query"},
            {"image_id": "img-003", "subject_id": None, "path": "query/c.png", "role": "query"},
            {"image_id": "img-004", "subject_id": "subject-1", "path": "query/d.png", "role": "query"},
        ],
    })


def test_runner_indexes_one_face_and_scopes_search() -> None:
    index = FakeIndex()
    content = {"img-001": b"1", "img-002": b"1", "img-003": b"0", "img-004": b"2"}
    runner = BenchmarkRunner(pipeline=FakePipeline(), index=index, load_bytes=lambda entry: content[entry.image_id], clock_ms=iter(range(20)).__next__)

    result = runner.run(manifest())

    assert len(index.records) == 1
    assert index.searches == [{"dataset_id": "dataset-v1", "event_id": "event-v1", "limit": 10, "score_threshold": None}]
    assert [item.status for item in result.observations] == ["ok", "no_face", "ambiguous"]
    assert result.observations[0].timings.vector_search_ms == 1
    assert result.observations[0].timings.end_to_end_ms == 3
    assert result.observations[1].timings.vector_search_ms is None
    assert result.observations[2].timings.vector_search_ms is None
    assert result.enrollment_failures == 0
    assert index.torn_down is True
