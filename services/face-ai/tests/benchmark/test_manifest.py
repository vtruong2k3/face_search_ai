from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any, Self

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
            {
                "image_id": "img-001",
                "subject_id": "subject-001",
                "path": "enrollment/001.jpg",
                "role": "enrollment",
                "sha256": hashlib.sha256(b"enrollment").hexdigest(),
            },
            {
                "image_id": "img-002",
                "subject_id": "subject-001",
                "path": "queries/002.jpg",
                "role": "query",
                "sha256": hashlib.sha256(b"known-query").hexdigest(),
            },
            {
                "image_id": "img-003",
                "subject_id": None,
                "path": "queries/003.jpg",
                "role": "query",
                "sha256": hashlib.sha256(b"impostor-query").hexdigest(),
            },
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


def test_manifest_preserves_role_relative_order_and_fingerprints_entry_order() -> None:
    value = valid_manifest()
    entries = value["entries"]
    assert isinstance(entries, list)
    entries[:] = [entries[1], entries[0], entries[2]]

    ordered = BenchmarkManifest.from_dict(value)
    original_fingerprint = ordered.fingerprint

    assert [entry.image_id for entry in ordered.enrollment_entries] == ["img-001"]
    assert [entry.image_id for entry in ordered.query_entries] == ["img-002", "img-003"]

    entries[:] = [entries[2], entries[1], entries[0]]
    reordered = BenchmarkManifest.from_dict(value)

    assert [entry.image_id for entry in reordered.query_entries] == ["img-003", "img-002"]
    assert reordered.fingerprint != original_fingerprint


def test_manifest_normalizes_entry_checksum_and_fingerprint_commits_to_it() -> None:
    value = valid_manifest()
    entries = value["entries"]
    assert isinstance(entries, list)
    first = entries[0]
    assert isinstance(first, dict)
    first["sha256"] = "A" * 64

    manifest = BenchmarkManifest.from_dict(value)
    original_fingerprint = manifest.fingerprint

    assert manifest.entries[0].sha256 == "a" * 64
    first["sha256"] = "b" * 64
    assert BenchmarkManifest.from_dict(value).fingerprint != original_fingerprint


@pytest.mark.parametrize("checksum", [None, 1, "", "a" * 63, "a" * 65, "g" * 64])
def test_manifest_rejects_invalid_entry_checksum(checksum: object) -> None:
    value = valid_manifest()
    entries = value["entries"]
    assert isinstance(entries, list)
    first = entries[0]
    assert isinstance(first, dict)
    first["sha256"] = checksum

    with pytest.raises(ManifestError, match="entry checksum"):
        BenchmarkManifest.from_dict(value)


def test_manifest_requires_entry_checksum() -> None:
    value = valid_manifest()
    entries = value["entries"]
    assert isinstance(entries, list)
    first = entries[0]
    assert isinstance(first, dict)
    del first["sha256"]

    with pytest.raises(ManifestError, match="missing or unknown"):
        BenchmarkManifest.from_dict(value)


def test_verify_dataset_accepts_matching_bytes(tmp_path: Path) -> None:
    root = tmp_path / "dataset"
    (root / "enrollment").mkdir(parents=True)
    (root / "queries").mkdir()
    (root / "enrollment" / "001.jpg").write_bytes(b"enrollment")
    (root / "queries" / "002.jpg").write_bytes(b"known-query")
    (root / "queries" / "003.jpg").write_bytes(b"impostor-query")

    BenchmarkManifest.from_dict(valid_manifest()).verify_dataset(root)


@pytest.mark.parametrize("failure", ["changed", "missing", "directory"])
def test_verify_dataset_sanitizes_invalid_dataset(
    tmp_path: Path, failure: str
) -> None:
    root = tmp_path / "private-dataset"
    (root / "enrollment").mkdir(parents=True)
    (root / "queries").mkdir()
    target = root / "enrollment" / "001.jpg"
    if failure == "changed":
        target.write_bytes(b"changed")
    elif failure == "directory":
        target.mkdir()
    (root / "queries" / "002.jpg").write_bytes(b"known-query")
    (root / "queries" / "003.jpg").write_bytes(b"impostor-query")
    manifest = BenchmarkManifest.from_dict(valid_manifest())

    with pytest.raises(ManifestError) as captured:
        manifest.verify_dataset(root)

    message = str(captured.value)
    assert message == "dataset verification failed"
    assert str(tmp_path) not in message
    assert "img-001" not in message
    assert "subject-001" not in message
    assert "changed" not in message


def test_verify_dataset_rejects_symlink_escape_with_sanitized_error(tmp_path: Path) -> None:
    root = tmp_path / "dataset"
    (root / "enrollment").mkdir(parents=True)
    (root / "queries").mkdir()
    outside = tmp_path / "outside.jpg"
    outside.write_bytes(b"enrollment")
    (root / "enrollment" / "001.jpg").symlink_to(outside)
    (root / "queries" / "002.jpg").write_bytes(b"known-query")
    (root / "queries" / "003.jpg").write_bytes(b"impostor-query")

    with pytest.raises(ManifestError, match="^dataset verification failed$"):
        BenchmarkManifest.from_dict(valid_manifest()).verify_dataset(root)


def test_verify_dataset_hashes_in_bounded_chunks(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    root = tmp_path / "dataset"
    (root / "enrollment").mkdir(parents=True)
    (root / "queries").mkdir()
    (root / "enrollment" / "001.jpg").write_bytes(b"enrollment")
    (root / "queries" / "002.jpg").write_bytes(b"known-query")
    (root / "queries" / "003.jpg").write_bytes(b"impostor-query")
    read_sizes: list[int] = []
    real_open = Path.open

    class TrackingReader:
        def __init__(self, wrapped: Any) -> None:
            self._wrapped = wrapped

        def __enter__(self) -> Self:
            self._wrapped.__enter__()
            return self

        def __exit__(self, *args: object) -> object:
            return self._wrapped.__exit__(*args)

        def read(self, size: int = -1) -> bytes:
            read_sizes.append(size)
            return self._wrapped.read(size)

    def tracking_open(path: Path, *args: Any, **kwargs: Any) -> TrackingReader:
        return TrackingReader(real_open(path, *args, **kwargs))

    monkeypatch.setattr(Path, "open", tracking_open)

    BenchmarkManifest.from_dict(valid_manifest()).verify_dataset(root)

    assert read_sizes
    assert -1 not in read_sizes
    assert all(0 < size <= 1024 * 1024 for size in read_sizes)


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
