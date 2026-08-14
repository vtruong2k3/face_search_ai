from __future__ import annotations

from dataclasses import asdict, replace

import pytest

from face_ai.benchmark.execution_policy import BenchmarkExecution


def test_execution_descriptor_has_exact_aggregate_policy() -> None:
    descriptor = BenchmarkExecution.enrollment_primed(warmup_inference_count=3)

    assert asdict(descriptor) == {
        "policy": "enrollment_primed_serial",
        "enrollment_order": "manifest",
        "query_order": "manifest",
        "warmup_source": "all_enrollment_inference",
        "warmup_inference_count": 3,
        "discarded_query_count": 0,
        "cold_start_measurement": "not_measured",
        "query_latency_scope": "load_process_and_optional_search",
        "query_throughput_scope": "summed_query_end_to_end",
    }


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("policy", "cold"),
        ("enrollment_order", "sorted"),
        ("query_order", "shuffled"),
        ("warmup_source", "first_query"),
        ("warmup_inference_count", -1),
        ("warmup_inference_count", True),
        ("discarded_query_count", 1),
        ("cold_start_measurement", "measured"),
        ("query_latency_scope", "pipeline_only"),
        ("query_throughput_scope", "concurrent_capacity"),
    ],
)
def test_execution_descriptor_rejects_unsupported_semantics(
    field: str, value: object
) -> None:
    descriptor = BenchmarkExecution.enrollment_primed(warmup_inference_count=1)

    with pytest.raises(ValueError):
        replace(descriptor, **{field: value})
