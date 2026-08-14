from __future__ import annotations

import math
import resource
import sys
import time
from collections.abc import Callable
from dataclasses import dataclass

_CPU_SCOPE = "benchmark_runner_process_user_and_system"
_PEAK_RSS_SCOPE = "post_run_process_lifetime_high_water"


@dataclass(frozen=True, slots=True)
class ProcessResourceSample:
    process_cpu_ms: float
    peak_rss_bytes: int

    def __post_init__(self) -> None:
        if (
            isinstance(self.process_cpu_ms, bool)
            or not isinstance(self.process_cpu_ms, (int, float))
            or not math.isfinite(self.process_cpu_ms)
            or self.process_cpu_ms < 0
        ):
            raise ValueError("process CPU time must be finite and non-negative")
        if (
            isinstance(self.peak_rss_bytes, bool)
            or not isinstance(self.peak_rss_bytes, int)
            or self.peak_rss_bytes < 0
        ):
            raise ValueError("peak RSS must be a non-negative integer")


@dataclass(frozen=True, slots=True)
class ProcessResourceEvidence:
    status: str
    cpu_time_scope: str
    process_cpu_ms: float | None
    peak_rss_scope: str
    peak_rss_bytes: int | None

    def __post_init__(self) -> None:
        if self.status not in {"available", "unavailable", "synthetic"}:
            raise ValueError("invalid process resource status")
        if self.cpu_time_scope != _CPU_SCOPE or self.peak_rss_scope != _PEAK_RSS_SCOPE:
            raise ValueError("invalid process resource scope")
        if self.status == "unavailable":
            if self.process_cpu_ms is not None or self.peak_rss_bytes is not None:
                raise ValueError("unavailable resource values must be null")
            return
        ProcessResourceSample(
            process_cpu_ms=self.process_cpu_ms,  # type: ignore[arg-type]
            peak_rss_bytes=self.peak_rss_bytes,  # type: ignore[arg-type]
        )

    @classmethod
    def unavailable(cls) -> ProcessResourceEvidence:
        return cls("unavailable", _CPU_SCOPE, None, _PEAK_RSS_SCOPE, None)


class ProcessResourceProbe:
    def __init__(
        self,
        *,
        process_time_ns: Callable[[], int] = time.process_time_ns,
        get_max_rss: Callable[[], int | float] | None = None,
        platform_name: str = sys.platform,
    ) -> None:
        self._process_time_ns = process_time_ns
        self._get_max_rss = get_max_rss or (
            lambda: resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
        )
        self._platform_name = platform_name

    def sample(self) -> ProcessResourceSample | None:
        if self._platform_name not in {"linux", "darwin"}:
            return None
        try:
            cpu_ms = self._process_time_ns() / 1_000_000
            raw_rss = self._get_max_rss()
            if isinstance(raw_rss, bool) or not isinstance(raw_rss, (int, float)):
                return None
            rss_bytes = raw_rss * 1024 if self._platform_name == "linux" else raw_rss
            if not math.isfinite(rss_bytes) or rss_bytes < 0 or not float(rss_bytes).is_integer():
                return None
            return ProcessResourceSample(cpu_ms, int(rss_bytes))
        except Exception:  # noqa: BLE001 -- resource failures become sanitized unavailable evidence
            return None


def resource_evidence(
    before: ProcessResourceSample | None,
    after: ProcessResourceSample | None,
) -> ProcessResourceEvidence:
    if before is None or after is None:
        return ProcessResourceEvidence.unavailable()
    cpu_delta = after.process_cpu_ms - before.process_cpu_ms
    if cpu_delta < 0:
        return ProcessResourceEvidence.unavailable()
    return ProcessResourceEvidence(
        "available",
        _CPU_SCOPE,
        cpu_delta,
        _PEAK_RSS_SCOPE,
        after.peak_rss_bytes,
    )
