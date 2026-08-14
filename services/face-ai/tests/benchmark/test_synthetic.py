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
    assert "embedding" not in content
    assert "query-" not in content
    assert str(tmp_path) not in content
