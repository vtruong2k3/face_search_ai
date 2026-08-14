from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Literal


@dataclass(frozen=True, slots=True)
class Candidate:
    subject_id: str
    score: float

    def __post_init__(self) -> None:
        if not self.subject_id.strip() or not math.isfinite(self.score):
            raise ValueError("candidate must have a subject ID and finite score")


@dataclass(frozen=True, slots=True)
class QueryObservation:
    query_id: str
    expected_subject_id: str | None
    candidates: tuple[Candidate, ...]
    status: Literal["ok", "no_face", "ambiguous"]
    latency_ms: float

    def __post_init__(self) -> None:
        if not self.query_id.strip() or not math.isfinite(self.latency_ms) or self.latency_ms < 0:
            raise ValueError("query observation is invalid")
        best: dict[str, Candidate] = {}
        for candidate in self.candidates:
            previous = best.get(candidate.subject_id)
            if previous is None or candidate.score > previous.score:
                best[candidate.subject_id] = candidate
        ordered = tuple(sorted(best.values(), key=lambda item: (-item.score, item.subject_id)))
        object.__setattr__(self, "candidates", ordered)


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
        latency_ms=latency_percentiles(tuple(item.latency_ms for item in observations)),
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
