from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any

_ALLOWED_FIELDS = {
    "benchmark_id",
    "mode",
    "dataset_id",
    "dataset_version",
    "model_id",
    "manifest_fingerprint",
    "query_count",
    "enrollment_failures",
    "metrics",
    "calibration",
    "status",
    "reproducibility",
}


def write_report(path: Path, report: dict[str, Any]) -> None:
    if not any(part in {"benchmark-results", "benchmark-output"} for part in path.parts):
        raise ValueError("report path must be under benchmark-results or benchmark-output")
    unsupported = set(report) - _ALLOWED_FIELDS
    if unsupported:
        raise ValueError("report contains unsupported fields")
    path.parent.mkdir(parents=True, exist_ok=True)
    content = json.dumps(report, sort_keys=True, separators=(",", ":"), ensure_ascii=True) + "\n"
    temporary = path.with_name(f".{path.name}.tmp")
    try:
        temporary.write_text(content, encoding="utf-8")
        os.replace(temporary, path)
    finally:
        temporary.unlink(missing_ok=True)
