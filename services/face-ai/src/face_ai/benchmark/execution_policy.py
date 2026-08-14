from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True, slots=True)
class BenchmarkExecution:
    policy: str
    enrollment_order: str
    query_order: str
    warmup_source: str
    warmup_inference_count: int
    discarded_query_count: int
    cold_start_measurement: str
    query_latency_scope: str
    query_throughput_scope: str

    def __post_init__(self) -> None:
        expected = {
            "policy": "enrollment_primed_serial",
            "enrollment_order": "manifest",
            "query_order": "manifest",
            "warmup_source": "all_enrollment_inference",
            "discarded_query_count": 0,
            "cold_start_measurement": "not_measured",
            "query_latency_scope": "load_process_and_optional_search",
            "query_throughput_scope": "summed_query_end_to_end",
        }
        for field, value in expected.items():
            if getattr(self, field) != value:
                raise ValueError(f"unsupported benchmark execution {field}")
        if (
            isinstance(self.warmup_inference_count, bool)
            or not isinstance(self.warmup_inference_count, int)
            or self.warmup_inference_count < 0
        ):
            raise ValueError("warm-up inference count must be a non-negative integer")

    @classmethod
    def enrollment_primed(cls, *, warmup_inference_count: int) -> BenchmarkExecution:
        return cls(
            policy="enrollment_primed_serial",
            enrollment_order="manifest",
            query_order="manifest",
            warmup_source="all_enrollment_inference",
            warmup_inference_count=warmup_inference_count,
            discarded_query_count=0,
            cold_start_measurement="not_measured",
            query_latency_scope="load_process_and_optional_search",
            query_throughput_scope="summed_query_end_to_end",
        )
