from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Literal

from face_ai.benchmark.manifest import ConditionSettings


@dataclass(frozen=True, slots=True)
class Candidate:
    subject_id: str
    score: float

    def __post_init__(self) -> None:
        if not self.subject_id.strip() or not math.isfinite(self.score):
            raise ValueError("candidate must have a subject ID and finite score")


@dataclass(frozen=True, slots=True)
class QueryTimings:
    decode_validation_ms: float
    detection_ms: float
    alignment_ms: float
    embedding_ms: float
    vector_search_ms: float | None
    end_to_end_ms: float

    def __post_init__(self) -> None:
        values = [
            self.decode_validation_ms,
            self.detection_ms,
            self.alignment_ms,
            self.embedding_ms,
            self.end_to_end_ms,
        ]
        if self.vector_search_ms is not None:
            values.append(self.vector_search_ms)
        if any(not math.isfinite(value) or value < 0 for value in values):
            raise ValueError("query timings must be finite and non-negative")


@dataclass(frozen=True, slots=True)
class QueryObservation:
    query_id: str
    expected_subject_id: str | None
    candidates: tuple[Candidate, ...]
    status: Literal["ok", "no_face", "ambiguous"]
    timings: QueryTimings
    conditions: tuple[tuple[str, str], ...] = ()

    def __post_init__(self) -> None:
        if not self.query_id.strip():
            raise ValueError("query observation is invalid")
        ordered_conditions = tuple(sorted(self.conditions))
        if len(ordered_conditions) != len({dimension for dimension, _ in ordered_conditions}):
            raise ValueError("query condition dimensions must be unique")
        if any(not dimension.strip() or not value.strip() for dimension, value in ordered_conditions):
            raise ValueError("query conditions are invalid")
        object.__setattr__(self, "conditions", ordered_conditions)
        best: dict[str, Candidate] = {}
        for candidate in self.candidates:
            previous = best.get(candidate.subject_id)
            if previous is None or candidate.score > previous.score:
                best[candidate.subject_id] = candidate
        ordered = tuple(sorted(best.values(), key=lambda item: (-item.score, item.subject_id)))
        object.__setattr__(self, "candidates", ordered)


@dataclass(frozen=True, slots=True)
class EnrollmentTiming:
    decode_validation_ms: float
    detection_ms: float
    alignment_ms: float
    embedding_ms: float
    end_to_end_ms: float

    def __post_init__(self) -> None:
        values = (
            self.decode_validation_ms,
            self.detection_ms,
            self.alignment_ms,
            self.embedding_ms,
            self.end_to_end_ms,
        )
        if any(not math.isfinite(value) or value < 0 for value in values):
            raise ValueError("enrollment timings must be finite and non-negative")


@dataclass(frozen=True, slots=True)
class VectorIndexTimings:
    setup_ms: float
    upsert_ms: float
    teardown_ms: float
    upsert_batch_size: int = 100

    def __post_init__(self) -> None:
        values = (self.setup_ms, self.upsert_ms, self.teardown_ms)
        if any(not math.isfinite(value) or value < 0 for value in values):
            raise ValueError("vector index timings must be finite and non-negative")
        if (
            isinstance(self.upsert_batch_size, bool)
            or not isinstance(self.upsert_batch_size, int)
            or self.upsert_batch_size <= 0
        ):
            raise ValueError("upsert batch size must be a positive integer")


@dataclass(frozen=True, slots=True)
class PerformanceResult:
    query_count: int
    vector_search_count: int
    latency_ms: dict[str, dict[str, float | None]]
    queries_per_second: float | None
    enrollment: dict[str, object]
    vector_index: dict[str, int | float]


def aggregate_performance(
    observations: tuple[QueryObservation, ...],
    *,
    enrollment_timings: tuple[EnrollmentTiming, ...] = (),
    indexed_vector_count: int = 0,
    vector_index_timings: VectorIndexTimings | None = None,
) -> PerformanceResult:
    timings = tuple(item.timings for item in observations)
    search_samples = tuple(
        item.vector_search_ms for item in timings if item.vector_search_ms is not None
    )
    end_to_end = tuple(item.end_to_end_ms for item in timings)
    total_ms = sum(end_to_end)
    throughput = len(timings) * 1000 / total_ms if timings and total_ms > 0 else None
    return PerformanceResult(
        query_count=len(timings),
        vector_search_count=len(search_samples),
        latency_ms={
            "decode_validation": latency_percentiles(tuple(item.decode_validation_ms for item in timings)),
            "detection": latency_percentiles(tuple(item.detection_ms for item in timings)),
            "alignment": latency_percentiles(tuple(item.alignment_ms for item in timings)),
            "embedding": latency_percentiles(tuple(item.embedding_ms for item in timings)),
            "vector_search": latency_percentiles(search_samples),
            "end_to_end": latency_percentiles(end_to_end),
        },
        queries_per_second=throughput,
        enrollment={
            "inference_count": len(enrollment_timings),
            "indexed_vector_count": indexed_vector_count,
            "inference_ms": {
                "decode_validation": latency_percentiles(tuple(item.decode_validation_ms for item in enrollment_timings)),
                "detection": latency_percentiles(tuple(item.detection_ms for item in enrollment_timings)),
                "alignment": latency_percentiles(tuple(item.alignment_ms for item in enrollment_timings)),
                "embedding": latency_percentiles(tuple(item.embedding_ms for item in enrollment_timings)),
                "end_to_end": latency_percentiles(tuple(item.end_to_end_ms for item in enrollment_timings)),
            },
            "total_inference_ms": sum(item.end_to_end_ms for item in enrollment_timings),
        },
        vector_index={
            "setup_ms": vector_index_timings.setup_ms if vector_index_timings else 0.0,
            "upsert_batch_size": vector_index_timings.upsert_batch_size if vector_index_timings else 100,
            "upserted_vector_count": indexed_vector_count,
            "upsert_ms": vector_index_timings.upsert_ms if vector_index_timings else 0.0,
            "teardown_ms": vector_index_timings.teardown_ms if vector_index_timings else 0.0,
        },
    )


@dataclass(frozen=True, slots=True)
class MetricResult:
    threshold: float
    query_count: int
    tp: int
    fp: int
    tn: int
    fn: int
    precision: float | None
    recall: float | None
    far: float | None
    frr: float | None
    top_k_accuracy: dict[int, float | None]
    no_face_rate: float | None
    ambiguous_rate: float | None
    latency_ms: dict[str, float | None]


@dataclass(frozen=True, slots=True)
class ConditionSliceResult:
    dimension: str
    value: str
    query_count: int
    status: Literal["available", "suppressed"]
    points: tuple[MetricResult, ...]


def aggregate_condition_slices(
    observations: tuple[QueryObservation, ...],
    *,
    settings: ConditionSettings | None,
    thresholds: tuple[float, ...],
    top_k: tuple[int, ...],
) -> tuple[ConditionSliceResult, ...]:
    if settings is None:
        return ()
    results: list[ConditionSliceResult] = []
    for dimension, values in settings.dimensions:
        for value in values:
            selected = tuple(
                observation
                for observation in observations
                if (dimension, value) in observation.conditions
            )
            available = len(selected) >= settings.minimum_slice_size
            points = (
                tuple(
                    calculate_metrics(selected, threshold=threshold, top_k=top_k)
                    for threshold in sorted(set(thresholds))
                )
                if available
                else ()
            )
            results.append(
                ConditionSliceResult(
                    dimension,
                    value,
                    len(selected),
                    "available" if available else "suppressed",
                    points,
                )
            )
    return tuple(results)


def calculate_metrics(
    observations: tuple[QueryObservation, ...], *, threshold: float, top_k: tuple[int, ...]
) -> MetricResult:
    if not math.isfinite(threshold) or not top_k or any(value <= 0 for value in top_k):
        raise ValueError("metric settings are invalid")
    tp = fp = tn = fn = 0
    evaluable_matches = [item for item in observations if item.expected_subject_id is not None]
    for item in observations:
        accepted = item.candidates[0] if item.status == "ok" and item.candidates and item.candidates[0].score >= threshold else None
        if item.expected_subject_id is None:
            if accepted is None:
                tn += 1
            else:
                fp += 1
        elif accepted is not None and accepted.subject_id == item.expected_subject_id:
            tp += 1
        else:
            fn += 1
            if accepted is not None:
                fp += 1

    top_accuracy: dict[int, float | None] = {}
    for k in sorted(set(top_k)):
        hits = sum(item.expected_subject_id in {candidate.subject_id for candidate in item.candidates[:k]} for item in evaluable_matches)
        top_accuracy[k] = _ratio(hits, len(evaluable_matches))
    total = len(observations)
    return MetricResult(
        threshold=threshold,
        query_count=total,
        tp=tp,
        fp=fp,
        tn=tn,
        fn=fn,
        precision=_ratio(tp, tp + fp),
        recall=_ratio(tp, tp + fn),
        far=_ratio(fp, fp + tn),
        frr=_ratio(fn, tp + fn),
        top_k_accuracy=top_accuracy,
        no_face_rate=_ratio(sum(item.status == "no_face" for item in observations), total),
        ambiguous_rate=_ratio(sum(item.status == "ambiguous" for item in observations), total),
        latency_ms=latency_percentiles(tuple(item.timings.end_to_end_ms for item in observations)),
    )


def latency_percentiles(samples: tuple[float, ...]) -> dict[str, float | None]:
    if not samples:
        return {name: None for name in ("p50", "p90", "p95", "p99")}
    if any(not math.isfinite(value) or value < 0 for value in samples):
        raise ValueError("latency samples must be finite and non-negative")
    ordered = sorted(samples)
    return {f"p{percent}": _percentile(ordered, percent / 100) for percent in (50, 90, 95, 99)}


def _percentile(values: list[float], quantile: float) -> float:
    index = (len(values) - 1) * quantile
    lower = math.floor(index)
    upper = math.ceil(index)
    if lower == upper:
        return values[lower]
    return values[lower] + (values[upper] - values[lower]) * (index - lower)


def _ratio(numerator: int, denominator: int) -> float | None:
    return numerator / denominator if denominator else None
