"""Privacy-safe metrics and a minimal health/metrics HTTP surface for the worker.

The photo worker is a background Redis consumer with no request surface of its
own, so this module adds a small, dependency-light HTTP server that exposes:

* ``GET /health/live``  — process liveness only (no dependency calls).
* ``GET /health/ready`` — readiness: whether the consumer has connected to Redis
  and joined its consumer group. Distinct from liveness by design.
* ``GET /metrics``      — Prometheus exposition for the worker's job metrics.

Every metric label is bounded and low-cardinality: the job type and a small set
of failure reasons. No label ever carries a photo id, event id, object path,
embedding, or any biometric/personal data. Correlation identifiers (job/photo id)
appear only in structured logs, never as metric labels.
"""

from __future__ import annotations

import threading
from collections.abc import Callable
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import structlog
from prometheus_client import CONTENT_TYPE_LATEST, Counter, Histogram, generate_latest

log = structlog.get_logger()

JOBS_PROCESSED = Counter(
    "photo_worker_jobs_processed_total",
    "Jobs processed successfully, labeled by bounded job type.",
    ["job_type"],
)
JOBS_FAILED = Counter(
    "photo_worker_jobs_failed_total",
    "Jobs that failed, labeled by bounded job type and failure reason.",
    ["job_type", "reason"],
)
JOBS_RETRIED = Counter(
    "photo_worker_jobs_retried_total",
    "Job processing attempts that failed and will be retried, labeled by job type.",
    ["job_type"],
)
JOBS_DEAD_LETTERED = Counter(
    "photo_worker_jobs_dead_lettered_total",
    "Jobs routed to the dead-letter stream.",
)
JOB_DURATION = Histogram(
    "photo_worker_job_duration_seconds",
    "End-to-end job processing latency in seconds, labeled by job type.",
    ["job_type"],
)


def record_processed(job_type: str, duration_seconds: float) -> None:
    JOBS_PROCESSED.labels(job_type=job_type).inc()
    JOB_DURATION.labels(job_type=job_type).observe(duration_seconds)


def record_retried(job_type: str) -> None:
    JOBS_RETRIED.labels(job_type=job_type).inc()


def record_failed(job_type: str, reason: str) -> None:
    JOBS_FAILED.labels(job_type=job_type, reason=reason).inc()


def record_dead_lettered() -> None:
    JOBS_DEAD_LETTERED.inc()


def _make_handler(is_ready: Callable[[], bool]) -> type[BaseHTTPRequestHandler]:
    class _Handler(BaseHTTPRequestHandler):
        def _write(self, status: int, body: bytes, content_type: str) -> None:
            self.send_response(status)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self) -> None:
            if self.path == "/health/live":
                # Liveness reflects only that the process is running; it never probes
                # Redis or any other dependency.
                self._write(200, b'{"status":"ok","service":"photo-worker"}', "application/json")
            elif self.path == "/health/ready":
                ready = is_ready()
                status = 200 if ready else 503
                body = b'{"status":"ready"}' if ready else b'{"status":"not_ready"}'
                self._write(status, body, "application/json")
            elif self.path == "/metrics":
                self._write(200, generate_latest(), CONTENT_TYPE_LATEST)
            else:
                self._write(404, b'{"status":"not_found"}', "application/json")

        def log_message(self, *_args: object) -> None:
            # Silence the default stderr access log; structured logs are emitted
            # elsewhere and access logs here would be noise.
            return

    return _Handler


class HealthServer:
    """Runs the health/metrics HTTP surface in a background daemon thread."""

    def __init__(self, host: str, port: int, is_ready: Callable[[], bool]) -> None:
        self._server = ThreadingHTTPServer((host, port), _make_handler(is_ready))
        self._host = host
        # The bound port is authoritative when port 0 (ephemeral) was requested.
        self._port = int(self._server.server_address[1])
        self._thread = threading.Thread(target=self._server.serve_forever, name="worker-health", daemon=True)

    def start(self) -> None:
        self._thread.start()
        log.info("worker_health_server_started", host=self._host, port=self._port)

    def stop(self) -> None:
        self._server.shutdown()
        self._server.server_close()
        log.info("worker_health_server_stopped")
