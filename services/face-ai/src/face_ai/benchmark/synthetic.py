from __future__ import annotations

from dataclasses import asdict
from pathlib import Path
from typing import Any

from face_ai.benchmark.calibration import CalibrationPolicy, calibrate
from face_ai.benchmark.metrics import (
    Candidate,
    QueryObservation,
    QueryTimings,
    aggregate_performance,
    calculate_metrics,
)
from face_ai.benchmark.report import write_report


def _timings(end_to_end_ms: float, *, searched: bool = True) -> QueryTimings:
    return QueryTimings(1.0, 2.0, 3.0, 4.0, 1.0 if searched else None, end_to_end_ms)


def synthetic_observations() -> tuple[QueryObservation, ...]:
    return (
        QueryObservation("query-1", "subject-1", (Candidate("subject-1", 0.9),), "ok", _timings(10.0)),
        QueryObservation("query-2", "subject-2", (Candidate("subject-3", 0.7), Candidate("subject-2", 0.6)), "ok", _timings(20.0)),
        QueryObservation("query-3", None, (Candidate("subject-4", 0.65),), "ok", _timings(30.0)),
        QueryObservation("query-4", None, (), "no_face", _timings(40.0, searched=False)),
    )


def run_synthetic(output: Path) -> dict[str, Any]:
    observations = synthetic_observations()
    policy = CalibrationPolicy(max_far=0.5, max_frr=0.5, min_recall=0.5)
    calibration = calibrate(observations, thresholds=(0.7, 0.8), top_k=(1, 2), policy=policy)
    if calibration.status != "recommended" or calibration.recommended_threshold != 0.8:
        raise RuntimeError("synthetic benchmark self-check failed")
    metrics = calculate_metrics(observations, threshold=0.8, top_k=(1, 2))
    if (metrics.tp, metrics.fp, metrics.tn, metrics.fn) != (1, 0, 2, 1):
        raise RuntimeError("synthetic benchmark self-check failed")

    report: dict[str, Any] = {
        "benchmark_id": "synthetic-v1",
        "mode": "synthetic_no_biometric_data",
        "dataset_id": "synthetic-dataset-v1",
        "dataset_version": "1",
        "model_id": "synthetic-model-v1",
        "manifest_fingerprint": "0" * 64,
        "query_count": len(observations),
        "enrollment_failures": 0,
        "metrics": asdict(metrics),
        "performance": asdict(aggregate_performance(observations)),
        "calibration": {
            "status": calibration.status,
            "recommended_threshold": calibration.recommended_threshold,
            "points": [asdict(point) for point in calibration.points],
        },
        "status": calibration.status,
        "reproducibility": {"fixture": "synthetic-v1", "volatile_fields": []},
    }
    write_report(output, report)
    return report
