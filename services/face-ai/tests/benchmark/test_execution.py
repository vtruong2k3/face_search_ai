from __future__ import annotations

import json
from pathlib import Path

import numpy as np
import pytest
from face_ai.benchmark.calibration import CalibrationPolicy
from face_ai.benchmark.execution import execute_benchmark
from face_ai.benchmark.manifest import BenchmarkManifest
from face_ai.domain import (
    BoundingBox,
    DetectedFace,
    EmbeddedFace,
    FacialLandmarks,
    SearchResult,
)
from face_ai.pipeline import PipelineResult
from face_ai.validation import ValidatedImage

_FACE = DetectedFace(
    BoundingBox(0, 0, 2, 2),
    FacialLandmarks((0, 0), (1, 0), (0.5, 0.5), (0, 1), (1, 1)),
    1.0,
)


class FakePipeline:
    def process(self, content: bytes) -> PipelineResult:
        count = int(content.decode())
        image = ValidatedImage("image/png", 2, 2, np.zeros((2, 2, 3), dtype=np.uint8))
        faces = tuple(
            EmbeddedFace(_FACE, np.array([1.0, 0.0], dtype=np.float32))
            for _ in range(count)
        )
        return PipelineResult(image, faces)


class FakeIndex:
    def __init__(self) -> None:
        self.torn_down = False

    def create(self) -> None:
        pass

    def upsert(self, records: object, *, batch_size: int = 100) -> None:
        pass

    def search(self, embedding: np.ndarray, **kwargs: object) -> list[SearchResult]:
        return [SearchResult("face-1", "enroll-1", 0.9)]

    def teardown(self) -> None:
        self.torn_down = True


def manifest() -> BenchmarkManifest:
    return BenchmarkManifest.from_dict(
        {
            "benchmark_id": "poc-v1",
            "mode": "personal_non_commercial_poc",
            "seed": 7,
            "dataset": {"id": "dataset-v1", "version": "v1", "event_id": "event-v1"},
            "model": {
                "id": "model-v1",
                "approval": "approved_non_commercial_poc",
                "detector_sha256": "a" * 64,
                "embedder_sha256": "b" * 64,
            },
            "search": {"limit": 5, "thresholds": [0.5, 0.95], "top_k": [1]},
            "entries": [
                {
                    "image_id": "enroll-1",
                    "subject_id": "subject-1",
                    "path": "enrollment/a.png",
                    "role": "enrollment",
                },
                {
                    "image_id": "query-1",
                    "subject_id": "subject-1",
                    "path": "queries/b.png",
                    "role": "query",
                },
                {
                    "image_id": "query-2",
                    "subject_id": None,
                    "path": "queries/c.png",
                    "role": "query",
                },
            ],
        }
    )


def test_execute_benchmark_writes_deterministic_aggregate_report(tmp_path: Path) -> None:
    dataset_root = tmp_path / "authorized"
    (dataset_root / "enrollment").mkdir(parents=True)
    (dataset_root / "queries").mkdir()
    (dataset_root / "enrollment" / "a.png").write_bytes(b"1")
    (dataset_root / "queries" / "b.png").write_bytes(b"1")
    (dataset_root / "queries" / "c.png").write_bytes(b"0")
    output = tmp_path / "benchmark-results" / "real.json"
    index = FakeIndex()
    clock = iter((0.0, 10.0, 20.0, 40.0)).__next__

    report = execute_benchmark(
        manifest=manifest(),
        dataset_root=dataset_root,
        output=output,
        pipeline=FakePipeline(),
        index=index,
        clock_ms=clock,
        policy=CalibrationPolicy(max_far=0.0, min_recall=0.5),
    )

    assert index.torn_down is True
    assert report["query_count"] == 2
    assert report["calibration"]["status"] == "recommended"
    assert report["calibration"]["recommended_threshold"] == 0.5
    assert [point["threshold"] for point in report["metrics"]] == [0.5, 0.95]
    assert report["reproducibility"]["seed"] == 7
    serialized = output.read_text(encoding="utf-8")
    assert str(dataset_root) not in serialized
    assert "query-1" not in serialized
    assert "subject-1" not in serialized
    assert "embedding" not in serialized
    assert json.loads(serialized) == report


def test_execute_benchmark_does_not_write_report_after_runner_failure(tmp_path: Path) -> None:
    dataset_root = tmp_path / "authorized"
    (dataset_root / "enrollment").mkdir(parents=True)
    (dataset_root / "queries").mkdir()
    (dataset_root / "enrollment" / "a.png").write_bytes(b"invalid")
    (dataset_root / "queries" / "b.png").write_bytes(b"1")
    (dataset_root / "queries" / "c.png").write_bytes(b"0")
    output = tmp_path / "benchmark-results" / "failed.json"
    index = FakeIndex()

    with pytest.raises(ValueError):
        execute_benchmark(
            manifest=manifest(),
            dataset_root=dataset_root,
            output=output,
            pipeline=FakePipeline(),
            index=index,
            clock_ms=lambda: 0.0,
            policy=CalibrationPolicy(max_far=0.0, min_recall=0.5),
        )

    assert index.torn_down is True
    assert not output.exists()
