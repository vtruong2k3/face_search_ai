from __future__ import annotations

import math

import pytest
from face_ai.benchmark.process_resources import (
    ProcessResourceEvidence,
    ProcessResourceProbe,
    ProcessResourceSample,
    resource_evidence,
)


def test_resource_evidence_reports_runner_cpu_delta_and_process_lifetime_peak() -> None:
    evidence = resource_evidence(
        ProcessResourceSample(process_cpu_ms=10.5, peak_rss_bytes=1_000),
        ProcessResourceSample(process_cpu_ms=14.0, peak_rss_bytes=2_000),
    )

    assert evidence == ProcessResourceEvidence(
        status="available",
        cpu_time_scope="benchmark_runner_process_user_and_system",
        process_cpu_ms=3.5,
        peak_rss_scope="post_run_process_lifetime_high_water",
        peak_rss_bytes=2_000,
    )


def test_resource_evidence_is_unavailable_without_two_samples() -> None:
    expected = ProcessResourceEvidence.unavailable()

    assert resource_evidence(None, ProcessResourceSample(2.0, 3)) == expected
    assert resource_evidence(ProcessResourceSample(1.0, 2), None) == expected
    assert resource_evidence(ProcessResourceSample(2.0, 2), ProcessResourceSample(1.0, 3)) == expected


@pytest.mark.parametrize("value", [math.nan, math.inf, -1.0, True])
def test_resource_sample_rejects_invalid_cpu_time(value: object) -> None:
    with pytest.raises(ValueError, match="process CPU time"):
        ProcessResourceSample(process_cpu_ms=value, peak_rss_bytes=1)  # type: ignore[arg-type]


@pytest.mark.parametrize("value", [-1, 1.5, True])
def test_resource_sample_rejects_invalid_peak_rss(value: object) -> None:
    with pytest.raises(ValueError, match="peak RSS"):
        ProcessResourceSample(process_cpu_ms=1.0, peak_rss_bytes=value)  # type: ignore[arg-type]


def test_probe_normalizes_linux_peak_rss_kib_to_bytes() -> None:
    probe = ProcessResourceProbe(
        process_time_ns=lambda: 1_500_000,
        get_max_rss=lambda: 123,
        platform_name="linux",
    )

    assert probe.sample() == ProcessResourceSample(1.5, 123 * 1024)


def test_probe_uses_byte_peak_rss_on_macos() -> None:
    probe = ProcessResourceProbe(
        process_time_ns=lambda: 2_000_000,
        get_max_rss=lambda: 456,
        platform_name="darwin",
    )

    assert probe.sample() == ProcessResourceSample(2.0, 456)


def test_probe_returns_none_for_unsupported_platform_or_sanitized_failure() -> None:
    unsupported = ProcessResourceProbe(
        process_time_ns=lambda: 1,
        get_max_rss=lambda: 1,
        platform_name="windows",
    )
    failing = ProcessResourceProbe(
        process_time_ns=lambda: (_ for _ in ()).throw(RuntimeError("private detail")),
        get_max_rss=lambda: 1,
        platform_name="linux",
    )

    assert unsupported.sample() is None
    assert failing.sample() is None
