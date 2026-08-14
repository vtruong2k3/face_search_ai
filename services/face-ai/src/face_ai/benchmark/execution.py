from __future__ import annotations

import json
from collections.abc import Callable
from dataclasses import asdict
from pathlib import Path
from typing import Any

from face_ai.benchmark.calibration import CalibrationPolicy, calibrate
from face_ai.benchmark.manifest import BenchmarkManifest, ManifestEntry
from face_ai.benchmark.metrics import aggregate_performance, calculate_metrics
from face_ai.benchmark.report import write_report
from face_ai.benchmark.runner import BenchmarkRunner, PipelinePort
from face_ai.benchmark.runtime_metadata import RuntimeMetadata
from face_ai.vector_store import VectorIndex


def execute_benchmark(
    *,
    manifest: BenchmarkManifest,
    dataset_root: Path,
    output: Path,
    pipeline: PipelinePort,
    index: VectorIndex,
    clock_ms: Callable[[], float],
    policy: CalibrationPolicy,
    runtime_metadata: RuntimeMetadata,
) -> dict[str, Any]:
    def load_bytes(entry: ManifestEntry) -> bytes:
        return manifest.resolve_image(dataset_root, entry).read_bytes()

    result = BenchmarkRunner(
        pipeline=pipeline,
        index=index,
        load_bytes=load_bytes,
        clock_ms=clock_ms,
    ).run(manifest)
    calibration = calibrate(
        result.observations,
        thresholds=manifest.search.thresholds,
        top_k=manifest.search.top_k,
        policy=policy,
    )
    metrics = tuple(
        calculate_metrics(
            result.observations,
            threshold=threshold,
            top_k=manifest.search.top_k,
        )
        for threshold in manifest.search.thresholds
    )
    performance = aggregate_performance(
        result.observations,
        enrollment_timings=result.enrollment_timings,
        indexed_vector_count=result.indexed_vector_count,
        vector_index_timings=result.vector_index_timings,
    )
    report: dict[str, Any] = {
        "benchmark_id": manifest.benchmark_id,
        "mode": manifest.mode.value,
        "dataset_id": manifest.dataset.id,
        "dataset_version": manifest.dataset.version,
        "model_id": manifest.model.id,
        "manifest_fingerprint": manifest.fingerprint,
        "query_count": len(result.observations),
        "enrollment_failures": result.enrollment_failures,
        "metrics": [asdict(point) for point in metrics],
        "performance": asdict(performance),
        "calibration": {
            "status": calibration.status,
            "recommended_threshold": calibration.recommended_threshold,
            "points": [asdict(point) for point in calibration.points],
        },
        "status": calibration.status,
        "reproducibility": {
            "seed": manifest.seed,
            "thresholds": list(manifest.search.thresholds),
            "top_k": list(manifest.search.top_k),
            "policy": asdict(policy),
            "runtime": asdict(runtime_metadata),
            "execution": asdict(result.execution),
        },
    }
    write_report(output, report)
    return json.loads(json.dumps(report, sort_keys=True))
