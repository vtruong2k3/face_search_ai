from __future__ import annotations

import json
from pathlib import Path

from face_ai.benchmark.cli import main


def manifest_value() -> dict[str, object]:
    return {
        "benchmark_id": "poc-v1",
        "mode": "personal_non_commercial_poc",
        "seed": 1,
        "dataset": {"id": "dataset-v1", "version": "v1", "event_id": "event-v1"},
        "model": {
            "id": "model-v1",
            "approval": "approved_non_commercial_poc",
            "detector_sha256": "a" * 64,
            "embedder_sha256": "b" * 64,
        },
        "search": {"limit": 5, "thresholds": [0.5], "top_k": [1]},
        "entries": [
            {"image_id": "enroll-1", "subject_id": "subject-1", "path": "enrollment/a.jpg", "role": "enrollment"},
            {"image_id": "query-1", "subject_id": "subject-1", "path": "queries/b.jpg", "role": "query"},
        ],
    }


def test_validate_command_reports_only_safe_summary(tmp_path: Path, capsys: object) -> None:
    root = tmp_path / "dataset"
    (root / "enrollment").mkdir(parents=True)
    (root / "queries").mkdir()
    (root / "enrollment" / "a.jpg").write_bytes(b"a")
    (root / "queries" / "b.jpg").write_bytes(b"b")
    manifest = tmp_path / "manifest.json"
    manifest.write_text(json.dumps(manifest_value()), encoding="utf-8")

    exit_code = main(["validate", "--manifest", str(manifest), "--dataset-root", str(root)])

    assert exit_code == 0
    output = capsys.readouterr().out  # type: ignore[attr-defined]
    assert "poc-v1" in output
    assert "enrollment=1" in output
    assert "queries=1" in output
    assert str(root) not in output


def test_validate_command_sanitizes_manifest_errors(tmp_path: Path, capsys: object) -> None:
    manifest = tmp_path / "manifest.json"
    manifest.write_text("not json", encoding="utf-8")

    exit_code = main(["validate", "--manifest", str(manifest), "--dataset-root", str(tmp_path)])

    assert exit_code == 2
    error = capsys.readouterr().err  # type: ignore[attr-defined]
    assert error == "benchmark validation failed\n"
    assert str(manifest) not in error
