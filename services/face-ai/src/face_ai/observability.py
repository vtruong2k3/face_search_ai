"""Privacy-safe Prometheus metrics for the internal Face AI service.

Metrics register on the default Prometheus registry, which the ``/metrics`` ASGI
app mounted in :mod:`face_ai.main` exposes. The ``/metrics`` surface is internal
only and is never routed through the public reverse proxy.

Every label is bounded and low-cardinality. No label ever carries an image, an
embedding, a face count, a raw error string, or any identifier. Diagnostic detail
such as face count belongs in structured logs, never in metric labels.
"""

from __future__ import annotations

from prometheus_client import Counter, Histogram

# Bounded outcome classes for inference requests. Keep this set small and stable.
INFERENCE_REQUESTS = Counter(
    "face_ai_inference_requests_total",
    "Internal inference requests handled, labeled by a bounded outcome class.",
    ["outcome"],
)

INFERENCE_DURATION = Histogram(
    "face_ai_inference_duration_seconds",
    "Internal inference request latency in seconds.",
)


def record_inference(outcome: str, duration_seconds: float) -> None:
    """Record one inference request's bounded outcome class and its latency."""
    INFERENCE_REQUESTS.labels(outcome=outcome).inc()
    INFERENCE_DURATION.observe(duration_seconds)
