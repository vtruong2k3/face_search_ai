from __future__ import annotations

import json
from pathlib import Path

from face_ai.benchmark.synthetic import run_synthetic


def test_synthetic_benchmark_writes_expected_aggregate_report(tmp_path: Path) -> None:
    output = tmp_path / "benchmark-results" / "synthetic.json"

    run_synthetic(output)
    content = output.read_text(encoding="utf-8")
    report = json.loads(content)
    run_synthetic(output)

    assert output.read_text(encoding="utf-8") == content
    assert report["status"] == "recommended"
    assert report["query_count"] == 4
    assert report["calibration"]["recommended_threshold"] == 0.8
    assert report["metrics"]["tp"] == 1
    assert report["metrics"]["fp"] == 0
    assert report["performance"]["vector_search_count"] == 3
    assert report["performance"]["queries_per_second"] == 40.0
    assert report["performance"]["enrollment"]["inference_count"] == 1
    assert report["performance"]["enrollment"]["total_inference_ms"] == 10.0
    assert report["performance"]["vector_index"] == {
        "setup_ms": 5.0,
        "upsert_batch_size": 100,
        "upserted_vector_count": 1,
        "upsert_ms": 6.0,
        "teardown_ms": 7.0,
    }
    assert report["reproducibility"]["runtime"]["execution_policy"] == "serial"
    assert report["reproducibility"]["runtime"]["operating_system"] == "synthetic"
    assert report["reproducibility"]["execution"]["policy"] == "enrollment_primed_serial"
    assert report["reproducibility"]["execution"]["warmup_inference_count"] == 0
    assert report["reproducibility"]["execution"]["cold_start_measurement"] == "not_measured"
    assert "subject-" not in content
    assert "query-" not in content
    assert str(tmp_path) not in content
