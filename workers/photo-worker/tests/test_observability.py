from __future__ import annotations

import urllib.request

from photo_worker import observability
from photo_worker.observability import HealthServer


def _get(port: int, path: str) -> tuple[int, str]:
    request = urllib.request.Request(f"http://127.0.0.1:{port}{path}", method="GET")
    try:
        with urllib.request.urlopen(request) as response:
            return response.status, response.read().decode("utf-8")
    except urllib.error.HTTPError as error:  # 503 raises HTTPError
        return error.code, error.read().decode("utf-8")


def test_liveness_is_independent_of_readiness() -> None:
    ready = False
    server = HealthServer("127.0.0.1", 0, lambda: ready)
    port = server._server.server_address[1]
    server.start()
    try:
        # Liveness is OK even while readiness is false: liveness must never depend on
        # dependency/consumer readiness.
        live_status, _ = _get(port, "/health/live")
        assert live_status == 200

        not_ready_status, not_ready_body = _get(port, "/health/ready")
        assert not_ready_status == 503
        assert "not_ready" in not_ready_body

        ready = True
        ready_status, ready_body = _get(port, "/health/ready")
        assert ready_status == 200
        assert "ready" in ready_body
    finally:
        server.stop()


def test_metrics_endpoint_exposes_bounded_job_series() -> None:
    observability.record_processed("photo.processing.requested", 0.01)
    observability.record_retried("photo.processing.requested")
    observability.record_failed("photo.processing.requested", "exhausted")
    observability.record_dead_lettered()

    server = HealthServer("127.0.0.1", 0, lambda: True)
    port = server._server.server_address[1]
    server.start()
    try:
        status, body = _get(port, "/metrics")
        assert status == 200
        assert "photo_worker_jobs_processed_total" in body
        assert "photo_worker_jobs_dead_lettered_total" in body
        assert 'reason="exhausted"' in body
        # No photo id, object path, or embedding is ever present as a label.
        assert "embedding" not in body
    finally:
        server.stop()
