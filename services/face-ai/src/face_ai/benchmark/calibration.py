from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Literal

from face_ai.benchmark.metrics import MetricResult, QueryObservation, calculate_metrics


@dataclass(frozen=True, slots=True)
class CalibrationPolicy:
    max_far: float
    min_recall: float
    max_frr: float | None = None

    def __post_init__(self) -> None:
        values = (self.max_far, self.min_recall) + (() if self.max_frr is None else (self.max_frr,))
        if any(not math.isfinite(value) or not 0 <= value <= 1 for value in values):
            raise ValueError("calibration policy rates must be between zero and one")


@dataclass(frozen=True, slots=True)
class CalibrationResult:
    status: Literal["recommended", "no_feasible_threshold"]
    points: tuple[MetricResult, ...]
    recommended_threshold: float | None


def calibrate(
    observations: tuple[QueryObservation, ...],
    *,
    thresholds: tuple[float, ...],
    top_k: tuple[int, ...],
    policy: CalibrationPolicy,
) -> CalibrationResult:
    if not thresholds or any(not math.isfinite(value) for value in thresholds):
        raise ValueError("calibration thresholds must be finite and non-empty")
    points = tuple(
        calculate_metrics(observations, threshold=threshold, top_k=top_k)
        for threshold in sorted(set(thresholds))
    )
    feasible = [point for point in points if _satisfies(point, policy)]
    if not feasible:
        return CalibrationResult("no_feasible_threshold", points, None)
    recommended = min(
        feasible,
        key=lambda point: (
            _undefined_worst(point.far),
            _undefined_worst(point.frr),
            -_undefined_zero(point.recall),
            -point.threshold,
        ),
    )
    return CalibrationResult("recommended", points, recommended.threshold)


def _satisfies(point: MetricResult, policy: CalibrationPolicy) -> bool:
    if point.far is None or point.recall is None:
        return False
    if point.far > policy.max_far or point.recall < policy.min_recall:
        return False
    return policy.max_frr is None or (point.frr is not None and point.frr <= policy.max_frr)


def _undefined_worst(value: float | None) -> float:
    return math.inf if value is None else value


def _undefined_zero(value: float | None) -> float:
    return 0.0 if value is None else value
