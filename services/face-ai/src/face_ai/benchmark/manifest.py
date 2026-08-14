from __future__ import annotations

import hashlib
import json
import math
import re
from dataclasses import dataclass
from enum import Enum
from pathlib import Path, PurePosixPath
from typing import Any


class ManifestError(ValueError):
    pass


class BenchmarkMode(str, Enum):
    PERSONAL_NON_COMMERCIAL_POC = "personal_non_commercial_poc"
    COMMERCIAL_EVALUATION = "commercial_evaluation"


class ImageRole(str, Enum):
    ENROLLMENT = "enrollment"
    QUERY = "query"


@dataclass(frozen=True, slots=True)
class DatasetMetadata:
    id: str
    version: str
    event_id: str


@dataclass(frozen=True, slots=True)
class ModelMetadata:
    id: str
    approval: str
    detector_sha256: str
    embedder_sha256: str


@dataclass(frozen=True, slots=True)
class SearchSettings:
    limit: int
    thresholds: tuple[float, ...]
    top_k: tuple[int, ...]


@dataclass(frozen=True, slots=True)
class ManifestEntry:
    image_id: str
    subject_id: str | None
    path: PurePosixPath
    role: ImageRole


@dataclass(frozen=True, slots=True)
class BenchmarkManifest:
    benchmark_id: str
    mode: BenchmarkMode
    seed: int
    dataset: DatasetMetadata
    model: ModelMetadata
    search: SearchSettings
    entries: tuple[ManifestEntry, ...]
    fingerprint: str

    @property
    def enrollment_entries(self) -> tuple[ManifestEntry, ...]:
        return tuple(entry for entry in self.entries if entry.role is ImageRole.ENROLLMENT)

    @property
    def query_entries(self) -> tuple[ManifestEntry, ...]:
        return tuple(entry for entry in self.entries if entry.role is ImageRole.QUERY)

    @classmethod
    def load(cls, path: Path) -> BenchmarkManifest:
        try:
            value = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            raise ManifestError("benchmark manifest must be valid readable JSON") from exc
        return cls.from_dict(value)

    @classmethod
    def from_dict(cls, value: object) -> BenchmarkManifest:
        root = _mapping(value, "manifest")
        _keys(root, {"benchmark_id", "mode", "seed", "dataset", "model", "search", "entries"}, "manifest")
        dataset_value = _mapping(root["dataset"], "dataset")
        _keys(dataset_value, {"id", "version", "event_id"}, "dataset")
        model_value = _mapping(root["model"], "model")
        _keys(model_value, {"id", "approval", "detector_sha256", "embedder_sha256"}, "model")
        search_value = _mapping(root["search"], "search")
        _keys(search_value, {"limit", "thresholds", "top_k"}, "search")

        try:
            mode = BenchmarkMode(_text(root["mode"], "mode"))
        except ValueError as exc:
            raise ManifestError("unsupported benchmark mode") from exc
        approval = _text(model_value["approval"], "model approval")
        if approval == "approved_non_commercial_poc" and mode is not BenchmarkMode.PERSONAL_NON_COMMERCIAL_POC:
            raise ManifestError("non-commercial model approval requires personal non-commercial mode")
        if approval not in {"approved_non_commercial_poc", "approved"}:
            raise ManifestError("model is not approved for benchmarking")

        entries_value = root["entries"]
        if not isinstance(entries_value, list) or not entries_value:
            raise ManifestError("entries must be a non-empty list")
        entries = tuple(_entry(item) for item in entries_value)
        image_ids = [entry.image_id for entry in entries]
        paths = [entry.path.as_posix() for entry in entries]
        if len(image_ids) != len(set(image_ids)):
            raise ManifestError("image IDs must be unique")
        if len(paths) != len(set(paths)):
            raise ManifestError("image paths must be unique across splits")
        if not any(entry.role is ImageRole.ENROLLMENT for entry in entries):
            raise ManifestError("manifest requires enrollment entries")
        if not any(entry.role is ImageRole.QUERY for entry in entries):
            raise ManifestError("manifest requires query entries")

        thresholds = _float_tuple(search_value["thresholds"], "thresholds")
        if any(value < -1.0 or value > 1.0 for value in thresholds):
            raise ManifestError("thresholds must be between -1 and 1")
        top_k = _int_tuple(search_value["top_k"], "top_k")
        limit = _positive_int(search_value["limit"], "search limit")
        if any(value > limit for value in top_k):
            raise ManifestError("top_k values must not exceed search limit")

        canonical = json.dumps(root, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
        return cls(
            benchmark_id=_opaque(root["benchmark_id"], "benchmark ID"),
            mode=mode,
            seed=_integer(root["seed"], "seed"),
            dataset=DatasetMetadata(
                id=_opaque(dataset_value["id"], "dataset ID"),
                version=_opaque(dataset_value["version"], "dataset version"),
                event_id=_opaque(dataset_value["event_id"], "event ID"),
            ),
            model=ModelMetadata(
                id=_opaque(model_value["id"], "model ID"),
                approval=approval,
                detector_sha256=_sha256(model_value["detector_sha256"], "detector checksum"),
                embedder_sha256=_sha256(model_value["embedder_sha256"], "embedder checksum"),
            ),
            search=SearchSettings(limit=limit, thresholds=tuple(sorted(set(thresholds))), top_k=tuple(sorted(set(top_k)))),
            entries=entries,
            fingerprint=hashlib.sha256(canonical.encode("utf-8")).hexdigest(),
        )

    def resolve_image(self, dataset_root: Path, entry: ManifestEntry) -> Path:
        root = dataset_root.resolve(strict=True)
        candidate = (root / Path(*entry.path.parts)).resolve(strict=True)
        if not candidate.is_relative_to(root):
            raise ManifestError("image path escapes dataset root")
        if not candidate.is_file():
            raise ManifestError("image path must resolve to a file")
        return candidate


def _entry(value: object) -> ManifestEntry:
    item = _mapping(value, "entry")
    _keys(item, {"image_id", "subject_id", "path", "role"}, "entry")
    try:
        role = ImageRole(_text(item["role"], "entry role"))
    except ValueError as exc:
        raise ManifestError("unsupported entry role") from exc
    raw_path = _text(item["path"], "image path")
    path = PurePosixPath(raw_path)
    if path.is_absolute() or ".." in path.parts or "." in path.parts:
        raise ManifestError("image paths must be safe relative paths")
    subject = item["subject_id"]
    if role is ImageRole.ENROLLMENT and subject is None:
        raise ManifestError("enrollment entries require a subject ID")
    return ManifestEntry(
        image_id=_opaque(item["image_id"], "image ID"),
        subject_id=None if subject is None else _opaque(subject, "subject ID"),
        path=path,
        role=role,
    )


def _mapping(value: object, name: str) -> dict[str, Any]:
    if not isinstance(value, dict) or not all(isinstance(key, str) for key in value):
        raise ManifestError(f"{name} must be an object")
    return value


def _keys(value: dict[str, Any], expected: set[str], name: str) -> None:
    if set(value) != expected:
        raise ManifestError(f"{name} has missing or unknown fields")


def _text(value: object, name: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise ManifestError(f"{name} must be non-empty text")
    return value.strip()


def _opaque(value: object, name: str) -> str:
    text = _text(value, name)
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,127}", text):
        raise ManifestError(f"{name} must be an opaque identifier")
    return text


def _sha256(value: object, name: str) -> str:
    text = _text(value, name).lower()
    if not re.fullmatch(r"[0-9a-f]{64}", text):
        raise ManifestError(f"{name} must be a SHA-256 digest")
    return text


def _integer(value: object, name: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int):
        raise ManifestError(f"{name} must be an integer")
    return value


def _positive_int(value: object, name: str) -> int:
    result = _integer(value, name)
    if result <= 0:
        raise ManifestError(f"{name} must be positive")
    return result


def _float_tuple(value: object, name: str) -> tuple[float, ...]:
    if not isinstance(value, list) or not value:
        raise ManifestError(f"{name} must be a non-empty list")
    result: list[float] = []
    for item in value:
        if isinstance(item, bool) or not isinstance(item, (int, float)) or not math.isfinite(item):
            raise ManifestError(f"{name} must contain finite numbers")
        result.append(float(item))
    return tuple(result)


def _int_tuple(value: object, name: str) -> tuple[int, ...]:
    if not isinstance(value, list) or not value:
        raise ManifestError(f"{name} must be a non-empty list")
    return tuple(_positive_int(item, name) for item in value)
