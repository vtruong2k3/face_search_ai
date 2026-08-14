from __future__ import annotations

import json
from pathlib import Path

import pytest
from face_ai.benchmark.report import write_report


def test_write_report_is_canonical_and_restricts_output_directory(tmp_path: Path) -> None:
    output = tmp_path / "benchmark-results" / "run.json"
    report = {"status": "no_feasible_threshold", "manifest_fingerprint": "a" * 64, "query_count": 3}

    write_report(output, report)

    assert output.read_text(encoding="utf-8") == json.dumps(report, sort_keys=True, separators=(",", ":")) + "\n"
    with pytest.raises(ValueError, match="benchmark-results"):
        write_report(tmp_path / "public" / "run.json", report)


def test_write_report_rejects_unknown_or_sensitive_fields(tmp_path: Path) -> None:
    with pytest.raises(ValueError, match="unsupported"):
        write_report(tmp_path / "benchmark-results" / "run.json", {"embedding": [1.0]})
