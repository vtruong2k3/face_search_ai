from __future__ import annotations

from io import BytesIO

import numpy as np
from fastapi.testclient import TestClient
from PIL import Image
from structlog.testing import capture_logs

from face_ai.domain import BoundingBox, DetectedFace, FacialLandmarks
from face_ai.main import create_app
from face_ai.pipeline import FacePipeline
from face_ai.routes.inference import get_pipeline
from face_ai.settings import Settings
from face_ai.validation import ImageValidationLimits


def make_image_bytes(size: tuple[int, int] = (32, 24), image_format: str = "PNG") -> bytes:
    image = Image.new("RGB", size, color=(20, 40, 60))
    buffer = BytesIO()
    image.save(buffer, format=image_format)
    return buffer.getvalue()


def _face(x: int, y: int, width: int, height: int, confidence: float) -> DetectedFace:
    return DetectedFace(
        bounding_box=BoundingBox(x=x, y=y, width=width, height=height),
        landmarks=FacialLandmarks(
            left_eye=(float(x + 1), float(y + 1)),
            right_eye=(float(x + width - 1), float(y + 1)),
            nose=(float(x + width / 2), float(y + height / 2)),
            left_mouth=(float(x + 1), float(y + height - 1)),
            right_mouth=(float(x + width - 1), float(y + height - 1)),
        ),
        confidence=confidence,
    )


class StubDetector:
    def __init__(self, faces: list[DetectedFace]) -> None:
        self._faces = faces

    def detect(self, image: np.ndarray) -> list[DetectedFace]:
        return list(self._faces)


class StubAligner:
    def align(self, image: np.ndarray, face: DetectedFace) -> np.ndarray:
        return np.zeros((4, 4, 3), dtype=np.uint8)


class StubEmbedder:
    dimension = 3

    def embed(self, aligned_face: np.ndarray) -> np.ndarray:
        return np.array([1.0, 0.0, 0.0], dtype=np.float32)


def build_stub_pipeline(
    faces: list[DetectedFace] | None = None,
    *,
    image_limits: ImageValidationLimits | None = None,
) -> FacePipeline:
    return FacePipeline(
        detector=StubDetector(faces if faces is not None else [_face(1, 1, 6, 6, 0.9)]),
        aligner=StubAligner(),
        embedder=StubEmbedder(),
        image_limits=image_limits,
    )


def make_client(pipeline: FacePipeline | None) -> TestClient:
    app = create_app()
    app.dependency_overrides[get_pipeline] = lambda: pipeline
    return TestClient(app)


def _post(client: TestClient, content: bytes) -> object:
    return client.post(
        "/internal/v1/extract-faces",
        content=content,
        headers={"content-type": "application/octet-stream"},
    )


def test_extract_faces_returns_typed_embeddings() -> None:
    client = make_client(build_stub_pipeline())

    response = _post(client, make_image_bytes())

    assert response.status_code == 200
    payload = response.json()
    assert payload["image_width"] == 32
    assert payload["image_height"] == 24
    assert payload["media_type"] == "image/png"
    assert payload["timings"]["detection_ms"] >= 0
    assert len(payload["faces"]) == 1

    face = payload["faces"][0]
    assert face["detection"]["bounding_box"] == {"x": 1, "y": 1, "width": 6, "height": 6}
    assert face["detection"]["confidence"] == 0.9
    assert len(face["embedding"]) == 3
    np.testing.assert_allclose(face["embedding"], [1.0, 0.0, 0.0])


def test_extract_faces_returns_all_faces_in_spatial_order() -> None:
    faces = [_face(1, 1, 6, 6, 0.9), _face(10, 2, 8, 8, 0.8)]
    client = make_client(build_stub_pipeline(faces))

    response = _post(client, make_image_bytes())

    assert response.status_code == 200
    assert [f["detection"]["bounding_box"]["x"] for f in response.json()["faces"]] == [1, 10]


def test_extract_faces_returns_503_when_pipeline_not_ready() -> None:
    client = make_client(None)

    response = _post(client, make_image_bytes())

    assert response.status_code == 503
    assert response.json()["detail"] == "inference_pipeline_not_ready"


def test_extract_faces_rejects_empty_body() -> None:
    client = make_client(build_stub_pipeline())

    response = _post(client, b"")

    assert response.status_code == 400
    assert "empty" in response.json()["detail"]


def test_extract_faces_rejects_corrupt_image() -> None:
    client = make_client(build_stub_pipeline())

    response = _post(client, b"not an image")

    assert response.status_code == 400


def test_extract_faces_rejects_oversized_payload() -> None:
    limits = ImageValidationLimits(max_bytes=10)
    client = make_client(build_stub_pipeline(image_limits=limits))

    response = _post(client, make_image_bytes())

    assert response.status_code == 413


def test_extract_faces_rejects_oversized_content_length() -> None:
    client = make_client(build_stub_pipeline())

    response = client.post(
        "/internal/v1/extract-faces",
        content=make_image_bytes(),
        headers={"content-type": "application/octet-stream", "content-length": str(21 * 1024 * 1024)},
    )

    assert response.status_code == 413


def test_extract_faces_requires_token_when_configured(monkeypatch) -> None:
    monkeypatch.setattr(
        "face_ai.security.get_settings",
        lambda: Settings(_env_file=None, internal_token="super-secret"),
    )
    client = make_client(build_stub_pipeline())
    image_bytes = make_image_bytes()

    missing = _post(client, image_bytes)
    assert missing.status_code == 401

    wrong = client.post(
        "/internal/v1/extract-faces",
        content=image_bytes,
        headers={"content-type": "application/octet-stream", "X-Internal-Token": "not-the-token"},
    )
    assert wrong.status_code == 403

    correct = client.post(
        "/internal/v1/extract-faces",
        content=image_bytes,
        headers={"content-type": "application/octet-stream", "X-Internal-Token": "super-secret"},
    )
    assert correct.status_code == 200


def test_metrics_endpoint_exposes_bounded_inference_outcomes() -> None:
    client = make_client(build_stub_pipeline())

    ok = _post(client, make_image_bytes())
    assert ok.status_code == 200

    not_ready_client = make_client(None)
    not_ready = _post(not_ready_client, make_image_bytes())
    assert not_ready.status_code == 503

    metrics = client.get("/metrics")
    assert metrics.status_code == 200
    body = metrics.text
    # Inference metrics are exposed and labeled only by a bounded outcome class.
    assert "face_ai_inference_requests_total" in body
    assert 'outcome="ok"' in body
    assert 'outcome="not_ready"' in body
    # No image, embedding, or identifier is ever present as a label.
    assert "embedding" not in body


def test_extract_faces_does_not_log_embeddings_or_image_bytes() -> None:
    client = make_client(build_stub_pipeline())
    image_bytes = make_image_bytes()

    with capture_logs() as captured:
        response = _post(client, image_bytes)

    assert response.status_code == 200
    assert captured, "expected at least one log record"

    serialized = repr(captured)
    # The normalized embedding and raw image bytes must never appear in logs.
    assert "[1.0, 0.0, 0.0]" not in serialized
    assert image_bytes.hex() not in serialized

    completed = [entry for entry in captured if entry.get("event") == "extract_faces_completed"]
    assert completed, "expected an extract_faces_completed log record"
    assert completed[0]["face_count"] == 1
    assert "embedding" not in completed[0]
