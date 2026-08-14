from __future__ import annotations

import pytest
from face_ai.benchmark.calibration import CalibrationPolicy, calibrate
from face_ai.benchmark.metrics import (
    Candidate,
    QueryObservation,
    QueryTimings,
    aggregate_performance,
    calculate_metrics,
    latency_percentiles,
)


def timing(end_to_end_ms: float, *, vector_search_ms: float | None = 1.0) -> QueryTimings:
    return QueryTimings(1.0, 2.0, 3.0, 4.0, vector_search_ms, end_to_end_ms)


def observations() -> tuple[QueryObservation, ...]:
    return (
        QueryObservation("q1", "s1", (Candidate("s1", 0.9), Candidate("s2", 0.8)), "ok", timing(10.0)),
        QueryObservation("q2", "s2", (Candidate("s3", 0.7), Candidate("s2", 0.6)), "ok", timing(20.0)),
        QueryObservation("q3", None, (Candidate("s4", 0.65),), "ok", timing(30.0)),
        QueryObservation("q4", None, (), "no_face", timing(40.0, vector_search_ms=None)),
    )


def test_metrics_use_inclusive_threshold_and_explicit_confusion_semantics() -> None:
    result = calculate_metrics(observations(), threshold=0.7, top_k=(1, 2))

    assert (result.tp, result.fp, result.tn, result.fn) == (1, 1, 2, 1)
    assert result.precision == 0.5
    assert result.recall == 0.5
    assert result.far == pytest.approx(1 / 3)
    assert result.frr == 0.5
    assert result.top_k_accuracy == {1: 0.5, 2: 1.0}
    assert result.no_face_rate == 0.25


def test_candidates_are_deduplicated_by_best_score_with_stable_ties() -> None:
    observation = QueryObservation(
        "q1", "s1", (Candidate("s2", 0.8), Candidate("s1", 0.7), Candidate("s1", 0.9), Candidate("s0", 0.8)), "ok", timing(1.0)
    )

    assert [(candidate.subject_id, candidate.score) for candidate in observation.candidates] == [
        ("s1", 0.9), ("s0", 0.8), ("s2", 0.8)
    ]


def test_undefined_rates_are_none() -> None:
    match = (QueryObservation("q", "s", (), "ok", timing(1.0)),)
    impostor = (QueryObservation("q", None, (), "ok", timing(1.0)),)

    assert calculate_metrics(match, threshold=0.5, top_k=(1,)).far is None
    assert calculate_metrics(impostor, threshold=0.5, top_k=(1,)).recall is None


def test_latency_percentiles_use_linear_interpolation() -> None:
    assert latency_percentiles((10.0, 20.0, 30.0, 40.0)) == pytest.approx(
        {"p50": 25.0, "p90": 37.0, "p95": 38.5, "p99": 39.7}
    )


def test_performance_aggregates_stages_search_samples_and_serial_throughput() -> None:
    result = aggregate_performance(observations())

    assert result.query_count == 4
    assert result.vector_search_count == 3
    assert result.latency_ms["end_to_end"]["p50"] == 25.0
    assert result.latency_ms["vector_search"]["p50"] == 1.0
    assert result.queries_per_second == 40.0


def test_performance_returns_no_throughput_for_zero_duration() -> None:
    zero = (QueryObservation("q", None, (), "no_face", timing(0.0, vector_search_ms=None)),)

    result = aggregate_performance(zero)

    assert result.queries_per_second is None
    assert result.vector_search_count == 0
    assert result.latency_ms["vector_search"]["p50"] is None


def test_calibration_is_sorted_deterministic_and_can_be_infeasible() -> None:
    policy = CalibrationPolicy(max_far=0.5, max_frr=0.5, min_recall=0.5)
    result = calibrate(observations(), thresholds=(0.8, 0.7, 0.7), top_k=(1, 2), policy=policy)

    assert [point.threshold for point in result.points] == [0.7, 0.8]
    assert result.status == "recommended"
    assert result.recommended_threshold == 0.8
    assert calibrate(
        observations(), thresholds=(0.95,), top_k=(1,), policy=CalibrationPolicy(max_far=0.0, max_frr=0.0, min_recall=1.0)
    ).status == "no_feasible_threshold"
