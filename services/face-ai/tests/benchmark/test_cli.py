from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any

from face_ai.benchmark.cli import RunDependencies, main
from face_ai.benchmark.runtime_metadata import RuntimeMetadata

_RUNTIME_METADATA = RuntimeMetadata(
    "TestOS", "test-arch", 4, "3.11.0", "CPUExecutionProvider", "1", "2",
    "3", "4", "5", "6", "buffalo_l", 640, 640, 0.5, "serial"
)


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
            {
                "image_id": "enroll-1",
                "subject_id": "subject-1",
                "path": "enrollment/a.jpg",
                "role": "enrollment",
                "sha256": hashlib.sha256(b"a").hexdigest(),
            },
            {
                "image_id": "query-1",
                "subject_id": "subject-1",
                "path": "queries/b.jpg",
                "role": "query",
                "sha256": hashlib.sha256(b"b").hexdigest(),
            },
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


def test_run_command_passes_explicit_policy_to_injected_executor(
    tmp_path: Path, capsys: object
) -> None:
    manifest = tmp_path / "manifest.json"
    manifest.write_text(json.dumps(manifest_value()), encoding="utf-8")
    captured: dict[str, Any] = {}
    pipeline = object()
    index = object()

    def execute(**kwargs: Any) -> dict[str, Any]:
        captured.update(kwargs)
        return {"status": "recommended"}

    dependencies = RunDependencies(
        verify_dataset=lambda loaded, root: None,
        verify_model=lambda loaded: None,
        get_runtime_metadata=lambda: _RUNTIME_METADATA,
        get_pipeline=lambda: pipeline,
        create_index=lambda loaded: index,
        execute=execute,
        clock_ms=lambda: 0.0,
    )
    output = tmp_path / "benchmark-results" / "run.json"

    exit_code = main(
        [
            "run",
            "--manifest",
            str(manifest),
            "--dataset-root",
            str(tmp_path),
            "--output",
            str(output),
            "--max-far",
            "0.01",
            "--min-recall",
            "0.9",
            "--max-frr",
            "0.2",
        ],
        run_dependencies=dependencies,
    )

    assert exit_code == 0
    assert captured["pipeline"] is pipeline
    assert captured["index"] is index
    assert captured["policy"].max_far == 0.01
    assert captured["policy"].min_recall == 0.9
    assert captured["policy"].max_frr == 0.2
    assert capsys.readouterr().out == "benchmark run completed\n"  # type: ignore[attr-defined]


def test_run_command_rejects_dataset_before_model_pipeline_and_index(
    tmp_path: Path, capsys: object
) -> None:
    manifest = tmp_path / "private-manifest.json"
    manifest.write_text(json.dumps(manifest_value()), encoding="utf-8")
    events: list[str] = []

    def reject_dataset(loaded: object, root: Path) -> None:
        events.append("dataset")
        raise RuntimeError(f"private mismatch at {root}")

    dependencies = RunDependencies(
        verify_dataset=reject_dataset,
        verify_model=lambda loaded: events.append("model"),
        get_runtime_metadata=lambda: events.append("metadata") or _RUNTIME_METADATA,
        get_pipeline=lambda: events.append("pipeline"),
        create_index=lambda loaded: events.append("index"),
        execute=lambda **kwargs: events.append("execute") or {},
        clock_ms=lambda: 0.0,
    )

    exit_code = main(
        [
            "run",
            "--manifest",
            str(manifest),
            "--dataset-root",
            str(tmp_path / "private-dataset"),
            "--output",
            str(tmp_path / "benchmark-results" / "run.json"),
            "--max-far",
            "0.01",
            "--min-recall",
            "0.9",
        ],
        run_dependencies=dependencies,
    )

    assert exit_code == 2
    assert events == ["dataset"]
    error = capsys.readouterr().err  # type: ignore[attr-defined]
    assert error == "benchmark run failed\n"
    assert str(tmp_path) not in error


def test_run_command_verifies_model_before_pipeline_and_index(
    tmp_path: Path, capsys: object
) -> None:
    manifest = tmp_path / "private-manifest.json"
    manifest.write_text(json.dumps(manifest_value()), encoding="utf-8")
    events: list[str] = []

    def reject_model(loaded: object) -> None:
        events.append("verify")
        raise RuntimeError(f"private mismatch at {tmp_path}")

    dependencies = RunDependencies(
        verify_dataset=lambda loaded, root: None,
        verify_model=reject_model,
        get_runtime_metadata=lambda: events.append("metadata") or _RUNTIME_METADATA,
        get_pipeline=lambda: events.append("pipeline"),
        create_index=lambda loaded: events.append("index"),
        execute=lambda **kwargs: events.append("execute") or {},
        clock_ms=lambda: 0.0,
    )

    exit_code = main(
        [
            "run",
            "--manifest",
            str(manifest),
            "--dataset-root",
            str(tmp_path / "private-dataset"),
            "--output",
            str(tmp_path / "benchmark-results" / "run.json"),
            "--max-far",
            "0.01",
            "--min-recall",
            "0.9",
        ],
        run_dependencies=dependencies,
    )

    assert exit_code == 2
    assert events == ["verify"]
    error = capsys.readouterr().err  # type: ignore[attr-defined]
    assert error == "benchmark run failed\n"
    assert str(tmp_path) not in error


def test_run_command_sanitizes_runtime_metadata_failure_before_pipeline(
    tmp_path: Path, capsys: object
) -> None:
    manifest = tmp_path / "private-manifest.json"
    manifest.write_text(json.dumps(manifest_value()), encoding="utf-8")
    events: list[str] = []

    def reject_metadata() -> RuntimeMetadata:
        events.append("metadata")
        raise RuntimeError(f"private runtime at {tmp_path}")

    dependencies = RunDependencies(
        verify_dataset=lambda loaded, root: events.append("dataset"),
        verify_model=lambda loaded: events.append("model"),
        get_runtime_metadata=reject_metadata,
        get_pipeline=lambda: events.append("pipeline"),
        create_index=lambda loaded: events.append("index"),
        execute=lambda **kwargs: events.append("execute") or {},
        clock_ms=lambda: 0.0,
    )

    exit_code = main(
        [
            "run", "--manifest", str(manifest), "--dataset-root", str(tmp_path),
            "--output", str(tmp_path / "benchmark-results" / "run.json"),
            "--max-far", "0.01", "--min-recall", "0.9",
        ],
        run_dependencies=dependencies,
    )

    assert exit_code == 2
    assert events == ["dataset", "model", "metadata"]
    assert capsys.readouterr().err == "benchmark run failed\n"  # type: ignore[attr-defined]


def test_run_command_sanitizes_unavailable_pipeline(
    tmp_path: Path, capsys: object
) -> None:
    manifest = tmp_path / "private-manifest.json"
    manifest.write_text(json.dumps(manifest_value()), encoding="utf-8")
    dependencies = RunDependencies(
        verify_dataset=lambda loaded, root: None,
        verify_model=lambda loaded: None,
        get_runtime_metadata=lambda: _RUNTIME_METADATA,
        get_pipeline=lambda: None,
        create_index=lambda loaded: (_ for _ in ()).throw(AssertionError("must not create index")),
        execute=lambda **kwargs: {},
        clock_ms=lambda: 0.0,
    )

    exit_code = main(
        [
            "run",
            "--manifest",
            str(manifest),
            "--dataset-root",
            str(tmp_path / "private-dataset"),
            "--output",
            str(tmp_path / "benchmark-results" / "run.json"),
            "--max-far",
            "0.01",
            "--min-recall",
            "0.9",
        ],
        run_dependencies=dependencies,
    )

    assert exit_code == 2
    error = capsys.readouterr().err  # type: ignore[attr-defined]
    assert error == "benchmark run failed\n"
    assert str(tmp_path) not in error
