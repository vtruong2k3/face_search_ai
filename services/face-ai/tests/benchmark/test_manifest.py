from __future__ import annotations

import json
from pathlib import Path

import pytest
from face_ai.benchmark.manifest import BenchmarkManifest, ManifestError


def valid_manifest() -> dict[str, object]:
    return {
        "benchmark_id": "poc-v1",
        "mode": "personal_non_commercial_poc",
        "seed": 20260814,
        "dataset": {"id": "dataset-v1", "version": "frozen-v1", "event_id": "event-v1"},
        "model": {
            "id": "buffalo-l-v1",
            "approval": "approved_non_commercial_poc",
            "detector_sha256": "a" * 64,
            "embedder_sha256": "b" * 64,
        },
        "search": {"limit": 10, "thresholds": [0.3, 0.5, 0.7], "top_k": [1, 5]},
        "entries": [
            {"image_id": "img-001", "subject_id": "subject-001", "path": "enrollment/001.jpg", "role": "enrollment"},
            {"image_id": "img-002", "subject_id": "subject-001", "path": "queries/002.jpg", "role": "query"},
            {"image_id": "img-003", "subject_id": None, "path": "queries/003.jpg", "role": "query"},
        ],
    }


def test_manifest_loads_strict_json_and_has_stable_fingerprint(tmp_path: Path) -> None:
    path = tmp_path / "manifest.json"
    path.write_text(json.dumps(valid_manifest()), encoding="utf-8")

    first = BenchmarkManifest.load(path)
    second = BenchmarkManifest.load(path)

    assert first.fingerprint == second.fingerprint
    assert len(first.fingerprint) == 64
    assert len(first.enrollment_entries) == 1
    assert len(first.query_entries) == 2


@pytest.mark.parametrize(
    "mutate",
    [
        lambda value: value["entries"].append(dict(value["entries"][0])),
        lambda value: value["entries"].append({**value["entries"][1], "image_id": "img-004"}),
        lambda value: value["entries"].__setitem__(0, {**value["entries"][0], "path": "../secret.jpg"}),
        lambda value: value["entries"].__setitem__(0, {**value["entries"][0], "path": "/tmp/secret.jpg"}),
        lambda value: value["model"].__setitem__("detector_sha256", "pending"),
    ],
)
def test_manifest_rejects_duplicate_or_unsafe_entries(mutate: object) -> None:
    value = valid_manifest()
    mutate(value)  # type: ignore[operator]

    with pytest.raises(ManifestError):
        BenchmarkManifest.from_dict(value)


def test_manifest_rejects_noncommercial_approval_outside_personal_mode() -> None:
    value = valid_manifest()
    value["mode"] = "commercial_evaluation"

    with pytest.raises(ManifestError, match="non-commercial"):
        BenchmarkManifest.from_dict(value)


def test_resolve_image_rejects_symlink_escape(tmp_path: Path) -> None:
    root = tmp_path / "dataset"
    root.mkdir()
    outside = tmp_path / "outside.jpg"
    outside.write_bytes(b"private")
    (root / "enrollment").mkdir()
    (root / "enrollment" / "001.jpg").symlink_to(outside)
    manifest = BenchmarkManifest.from_dict(valid_manifest())

    with pytest.raises(ManifestError, match="escapes"):
        manifest.resolve_image(root, manifest.enrollment_entries[0])
