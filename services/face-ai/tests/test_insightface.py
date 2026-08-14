from __future__ import annotations

from pathlib import Path
from types import SimpleNamespace
from typing import Any

import numpy as np
import pytest
from face_ai.models.insightface import (
    InsightFaceAligner,
    InsightFaceDetector,
    InsightFaceEmbedder,
    prepare_buffalo_l,
    resolve_buffalo_l_artifacts,
)


class FakeAnalysis:
    def __init__(self, faces: list[Any] | None = None) -> None:
        self.faces = faces or []
        self.received: np.ndarray | None = None
        self.prepared: dict[str, Any] | None = None
        self.models: dict[str, Any] = {}

    def prepare(self, **kwargs: Any) -> None:
        self.prepared = kwargs

    def get(self, image: np.ndarray) -> list[Any]:
        self.received = image
        return self.faces


class FakeRecognition:
    def __init__(self, output: np.ndarray) -> None:
        self.output = output
        self.received: np.ndarray | None = None

    def get_feat(self, image: np.ndarray) -> np.ndarray:
        self.received = image
        return self.output


def face(*, bbox: list[float] | None = None) -> Any:
    return SimpleNamespace(
        bbox=np.asarray(bbox or [1.2, 2.4, 8.8, 9.6], dtype=np.float32),
        kps=np.asarray(
            [[2.0, 4.0], [7.0, 4.0], [4.5, 6.0], [3.0, 8.0], [6.0, 8.0]],
            dtype=np.float32,
        ),
        det_score=0.95,
    )


def test_detector_converts_rgb_to_bgr_and_maps_face_geometry() -> None:
    app = FakeAnalysis([face(bbox=[-2.0, 1.0, 12.0, 9.0])])
    detector = InsightFaceDetector(app)
    image = np.zeros((10, 10, 3), dtype=np.uint8)
    image[0, 0] = [10, 20, 30]

    detections = detector.detect(image)

    assert app.received is not None
    assert app.received[0, 0].tolist() == [30, 20, 10]
    assert detections[0].bounding_box.x == 0
    assert detections[0].bounding_box.y == 1
    assert detections[0].bounding_box.width == 10
    assert detections[0].bounding_box.height == 8
    assert detections[0].landmarks.left_eye == (2.0, 4.0)
    assert detections[0].landmarks.right_mouth == (6.0, 8.0)


def test_detector_rejects_malformed_landmarks() -> None:
    malformed = face()
    malformed.kps = np.zeros((4, 2), dtype=np.float32)

    with pytest.raises(ValueError, match="landmarks"):
        InsightFaceDetector(FakeAnalysis([malformed])).detect(np.zeros((10, 10, 3), dtype=np.uint8))


def test_aligner_uses_five_landmarks_and_returns_rgb_112_crop() -> None:
    calls: list[tuple[np.ndarray, np.ndarray, int]] = []

    def align(image: np.ndarray, landmarks: np.ndarray, image_size: int) -> np.ndarray:
        calls.append((image, landmarks, image_size))
        crop = np.zeros((112, 112, 3), dtype=np.uint8)
        crop[0, 0] = [30, 20, 10]
        return crop

    detection = InsightFaceDetector(FakeAnalysis([face()])).detect(
        np.zeros((10, 10, 3), dtype=np.uint8)
    )[0]
    image = np.zeros((10, 10, 3), dtype=np.uint8)
    image[0, 0] = [10, 20, 30]

    result = InsightFaceAligner(align).align(image, detection)

    assert calls[0][0][0, 0].tolist() == [30, 20, 10]
    assert calls[0][1].shape == (5, 2)
    assert calls[0][2] == 112
    assert result.shape == (112, 112, 3)
    assert result[0, 0].tolist() == [10, 20, 30]


def test_embedder_returns_one_finite_512d_float32_vector() -> None:
    recognition = FakeRecognition(np.ones((1, 512), dtype=np.float64))
    embedder = InsightFaceEmbedder(recognition)
    image = np.zeros((112, 112, 3), dtype=np.uint8)
    image[0, 0] = [10, 20, 30]

    result = embedder.embed(image)

    assert recognition.received is not None
    assert recognition.received[0, 0].tolist() == [30, 20, 10]
    assert result.shape == (512,)
    assert result.dtype == np.float32


@pytest.mark.parametrize(
    "output",
    [np.ones((511,), dtype=np.float32), np.full((512,), np.nan, dtype=np.float32)],
)
def test_embedder_rejects_invalid_output(output: np.ndarray) -> None:
    with pytest.raises(ValueError, match="512|finite"):
        InsightFaceEmbedder(FakeRecognition(output)).embed(
            np.zeros((112, 112, 3), dtype=np.uint8)
        )


def test_resolve_buffalo_l_artifacts_uses_canonical_model_roles(tmp_path: Path) -> None:
    pack = tmp_path / "models" / "buffalo_l"
    pack.mkdir(parents=True)
    detector = pack / "det_10g.onnx"
    embedder = pack / "w600k_r50.onnx"
    detector.write_bytes(b"detector")
    embedder.write_bytes(b"embedder")

    artifacts = resolve_buffalo_l_artifacts(tmp_path)

    assert artifacts.detector == detector
    assert artifacts.embedder == embedder


@pytest.mark.parametrize("missing_name", ["det_10g.onnx", "w600k_r50.onnx"])
def test_resolve_buffalo_l_artifacts_requires_each_canonical_role(
    tmp_path: Path, missing_name: str
) -> None:
    pack = tmp_path / "models" / "buffalo_l"
    pack.mkdir(parents=True)
    for name in ("det_10g.onnx", "w600k_r50.onnx"):
        if name != missing_name:
            (pack / name).write_bytes(name.encode())

    with pytest.raises(RuntimeError, match="artifacts are unavailable"):
        resolve_buffalo_l_artifacts(tmp_path)


def test_resolve_buffalo_l_artifacts_rejects_symlink_escape(tmp_path: Path) -> None:
    pack = tmp_path / "models" / "buffalo_l"
    pack.mkdir(parents=True)
    outside = tmp_path / "outside.onnx"
    outside.write_bytes(b"detector")
    (pack / "det_10g.onnx").symlink_to(outside)
    (pack / "w600k_r50.onnx").write_bytes(b"embedder")

    with pytest.raises(RuntimeError, match="artifacts are unavailable"):
        resolve_buffalo_l_artifacts(tmp_path)


def test_prepare_requires_local_pack_before_factory_call(tmp_path: Path) -> None:
    called = False

    def factory(**kwargs: Any) -> FakeAnalysis:
        nonlocal called
        called = True
        return FakeAnalysis()

    with pytest.raises(RuntimeError, match="unavailable"):
        prepare_buffalo_l(model_root=tmp_path, analysis_factory=factory)

    assert called is False


def test_prepare_uses_cpu_without_downloading(tmp_path: Path) -> None:
    pack = tmp_path / "models" / "buffalo_l"
    pack.mkdir(parents=True)
    (pack / "det_10g.onnx").write_bytes(b"detector")
    (pack / "w600k_r50.onnx").write_bytes(b"embedder")
    app = FakeAnalysis()
    app.models["recognition"] = FakeRecognition(np.ones((1, 512), dtype=np.float32))
    captured: dict[str, Any] = {}

    def factory(**kwargs: Any) -> FakeAnalysis:
        captured.update(kwargs)
        return app

    adapters = prepare_buffalo_l(
        model_root=tmp_path,
        analysis_factory=factory,
        align=lambda image, landmarks, size: np.zeros((size, size, 3), dtype=np.uint8),
    )

    assert captured == {
        "name": "buffalo_l",
        "root": str(tmp_path),
        "providers": ["CPUExecutionProvider"],
    }
    assert app.prepared == {"ctx_id": -1, "det_size": (640, 640), "det_thresh": 0.5}
    assert adapters.embedder.dimension == 512
