from __future__ import annotations

import json
from pathlib import Path

import numpy as np
import pytest
from face_ai.benchmark.calibration import CalibrationPolicy
from face_ai.benchmark.execution import execute_benchmark
from face_ai.benchmark.manifest import BenchmarkManifest
from face_ai.benchmark.process_resources import ProcessResourceSample
from face_ai.benchmark.runtime_metadata import RuntimeMetadata
from face_ai.domain import (
    BoundingBox,
    DetectedFace,
    EmbeddedFace,
    FacialLandmarks,
    SearchResult,
)
from face_ai.pipeline import PipelineResult, PipelineTimings
from face_ai.validation import ValidatedImage

_RUNTIME_METADATA = RuntimeMetadata(
    "TestOS", "test-arch", 4, "3.11.0", "CPUExecutionProvider", "1", "2",
    "3", "4", "5", "6", "buffalo_l", 640, 640, 0.5, "serial"
)

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
        return PipelineResult(image, faces, PipelineTimings(1.0, 2.0, 3.0, 4.0))


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
                    "sha256": "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b",
                },
                {
                    "image_id": "query-1",
                    "subject_id": "subject-1",
                    "path": "queries/b.png",
                    "role": "query",
                    "sha256": "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b",
                },
                {
                    "image_id": "query-2",
                    "subject_id": None,
                    "path": "queries/c.png",
                    "role": "query",
                    "sha256": "5feceb66ffc86f38d952786c6d696c79c2dbc239dd4e91b46729d73a27fb57e9",
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
    clock = iter(
        (0.0, 5.0, 10.0, 20.0, 25.0, 31.0, 40.0, 45.0, 46.0, 50.0, 60.0, 70.0, 80.0, 87.0)
    ).__next__

    resource_samples = iter(
        (
            ProcessResourceSample(100.0, 1_000),
            ProcessResourceSample(125.5, 2_000),
        )
    )

    report = execute_benchmark(
        manifest=manifest(),
        dataset_root=dataset_root,
        output=output,
        pipeline=FakePipeline(),
        index=index,
        clock_ms=clock,
        policy=CalibrationPolicy(max_far=0.0, min_recall=0.5),
        runtime_metadata=_RUNTIME_METADATA,
        resource_sample=resource_samples.__next__,
    )

    assert index.torn_down is True
    assert report["query_count"] == 2
    assert report["calibration"]["status"] == "recommended"
    assert report["calibration"]["recommended_threshold"] == 0.5
    assert [point["threshold"] for point in report["metrics"]] == [0.5, 0.95]
    assert report["reproducibility"]["seed"] == 7
    assert report["reproducibility"]["runtime"]["architecture"] == "test-arch"
    assert report["reproducibility"]["execution"] == {
        "policy": "enrollment_primed_serial",
        "enrollment_order": "manifest",
        "query_order": "manifest",
        "warmup_source": "all_enrollment_inference",
        "warmup_inference_count": 1,
        "discarded_query_count": 0,
        "cold_start_measurement": "not_measured",
        "query_latency_scope": "load_process_and_optional_search",
        "query_throughput_scope": "summed_query_end_to_end",
    }
    assert report["performance"]["enrollment"] == {
        "inference_count": 1,
        "indexed_vector_count": 1,
        "inference_ms": {
            stage: {"p50": value, "p90": value, "p95": value, "p99": value}
            for stage, value in {
                "decode_validation": 1.0,
                "detection": 2.0,
                "alignment": 3.0,
                "embedding": 4.0,
                "end_to_end": 10.0,
            }.items()
        },
        "total_inference_ms": 10.0,
    }
    assert report["performance"]["vector_index"] == {
        "setup_ms": 5.0,
        "upsert_batch_size": 100,
        "upserted_vector_count": 1,
        "upsert_ms": 6.0,
        "teardown_ms": 7.0,
    }
    assert report["performance"]["process_resources"] == {
        "status": "available",
        "cpu_time_scope": "benchmark_runner_process_user_and_system",
        "process_cpu_ms": 25.5,
        "peak_rss_scope": "post_run_process_lifetime_high_water",
        "peak_rss_bytes": 2_000,
    }
    serialized = output.read_text(encoding="utf-8")
    assert str(dataset_root) not in serialized
    assert "query-1" not in serialized
    assert "subject-1" not in serialized
    assert "raw_timings" not in serialized
    assert json.loads(serialized) == report


def test_execute_benchmark_repeats_identical_logical_and_canonical_results(
    tmp_path: Path,
) -> None:
    dataset_root = tmp_path / "authorized"
    (dataset_root / "enrollment").mkdir(parents=True)
    (dataset_root / "queries").mkdir()
    (dataset_root / "enrollment" / "a.png").write_bytes(b"1")
    (dataset_root / "queries" / "b.png").write_bytes(b"1")
    (dataset_root / "queries" / "c.png").write_bytes(b"0")
    outputs = (
        tmp_path / "benchmark-results" / "first.json",
        tmp_path / "benchmark-results" / "second.json",
    )
    reports: list[dict[str, object]] = []

    for output in outputs:
        index = FakeIndex()
        reports.append(
            execute_benchmark(
                manifest=manifest(),
                dataset_root=dataset_root,
                output=output,
                pipeline=FakePipeline(),
                index=index,
                clock_ms=iter(
                    (0.0, 5.0, 10.0, 20.0, 25.0, 31.0, 40.0, 45.0, 46.0, 50.0, 60.0, 70.0, 80.0, 87.0)
                ).__next__,
                policy=CalibrationPolicy(max_far=0.0, min_recall=0.5),
                runtime_metadata=_RUNTIME_METADATA,
                resource_sample=iter(
                    (
                        ProcessResourceSample(100.0, 1_000),
                        ProcessResourceSample(125.5, 2_000),
                    )
                ).__next__,
            )
        )
        assert index.torn_down is True

    assert reports[0] == reports[1]
    assert outputs[0].read_bytes() == outputs[1].read_bytes()
    assert reports[0]["query_count"] == 2
    assert reports[0]["enrollment_failures"] == 0
    assert reports[0]["calibration"] == reports[1]["calibration"]
    assert reports[0]["performance"] == reports[1]["performance"]
    assert reports[0]["reproducibility"] == reports[1]["reproducibility"]

    serialized = outputs[0].read_text(encoding="utf-8")
    for private_value in (
        str(dataset_root),
        "query-1",
        "query-2",
        "subject-1",
        "enrollment/a.png",
        "queries/b.png",
        "raw_timings",
    ):
        assert private_value not in serialized


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
            runtime_metadata=_RUNTIME_METADATA,
            resource_sample=lambda: None,
        )

    assert index.torn_down is True
    assert not output.exists()
