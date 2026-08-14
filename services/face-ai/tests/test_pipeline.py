from __future__ import annotations

from io import BytesIO

import numpy as np
import pytest
from face_ai.domain import (
    BoundingBox,
    DetectedFace,
    EmbeddedFace,
    FaceInferenceError,
    FacialLandmarks,
)
from face_ai.pipeline import FacePipeline, PipelineTimings, normalize_embedding
from face_ai.validation import (
    ImageValidationError,
    ImageValidationLimits,
    validate_image,
)
from PIL import Image


def make_image_bytes(*, size: tuple[int, int] = (32, 24), image_format: str = "PNG") -> bytes:
    image = Image.new("RGB", size, color=(20, 40, 60))
    buffer = BytesIO()
    image.save(buffer, format=image_format)
    return buffer.getvalue()


class StubDetector:
    def detect(self, image: np.ndarray) -> list[DetectedFace]:
        assert image.shape == (24, 32, 3)
        return [
            DetectedFace(
                bounding_box=BoundingBox(x=10, y=2, width=8, height=8),
                landmarks=FacialLandmarks(
                    left_eye=(12.0, 4.0),
                    right_eye=(16.0, 4.0),
                    nose=(14.0, 6.0),
                    left_mouth=(12.5, 8.0),
                    right_mouth=(15.5, 8.0),
                ),
                confidence=0.95,
            ),
            DetectedFace(
                bounding_box=BoundingBox(x=1, y=1, width=6, height=6),
                landmarks=FacialLandmarks(
                    left_eye=(2.0, 2.0),
                    right_eye=(5.0, 2.0),
                    nose=(3.5, 4.0),
                    left_mouth=(2.5, 5.0),
                    right_mouth=(4.5, 5.0),
                ),
                confidence=0.8,
            ),
        ]


class StubAligner:
    def align(self, image: np.ndarray, face: DetectedFace) -> np.ndarray:
        return np.full((4, 4, 3), face.bounding_box.x, dtype=np.uint8)


class StubEmbedder:
    @property
    def dimension(self) -> int:
        return 3

    def embed(self, aligned_face: np.ndarray) -> np.ndarray:
        marker = float(aligned_face[0, 0, 0])
        return np.array([marker, 0.0, marker], dtype=np.float32)


def test_validate_image_decodes_rgb_pixels() -> None:
    validated = validate_image(make_image_bytes())

    assert validated.media_type == "image/png"
    assert validated.width == 32
    assert validated.height == 24
    assert validated.pixels.shape == (24, 32, 3)
    assert validated.pixels.dtype == np.uint8


@pytest.mark.parametrize(
    ("content", "message"),
    [
        (b"", "empty"),
        (b"not an image", "unsupported or corrupt"),
    ],
)
def test_validate_image_rejects_malformed_content(content: bytes, message: str) -> None:
    with pytest.raises(ImageValidationError, match=message):
        validate_image(content)


def test_validate_image_rejects_payload_over_byte_limit() -> None:
    with pytest.raises(ImageValidationError, match="byte limit"):
        validate_image(make_image_bytes(), limits=ImageValidationLimits(max_bytes=10))


def test_validate_image_rejects_dimensions_over_limit() -> None:
    limits = ImageValidationLimits(max_width=16, max_height=16, max_pixels=256)

    with pytest.raises(ImageValidationError, match="dimensions"):
        validate_image(make_image_bytes(size=(32, 24)), limits=limits)


def test_normalize_embedding_returns_unit_float32_vector() -> None:
    normalized = normalize_embedding(np.array([3.0, 4.0], dtype=np.float64))

    np.testing.assert_allclose(normalized, np.array([0.6, 0.8], dtype=np.float32))
    assert normalized.dtype == np.float32
    assert normalized.flags.writeable is False


@pytest.mark.parametrize(
    "embedding",
    [
        np.array([], dtype=np.float32),
        np.array([[1.0, 2.0]], dtype=np.float32),
        np.array([0.0, 0.0], dtype=np.float32),
        np.array([1.0, np.nan], dtype=np.float32),
    ],
)
def test_normalize_embedding_rejects_invalid_vectors(embedding: np.ndarray) -> None:
    with pytest.raises(ValueError):
        normalize_embedding(embedding)


def test_pipeline_records_single_pass_stage_totals() -> None:
    clock = iter((0.0, 1.0, 2.0, 4.0, 5.0, 8.0, 9.0, 13.0, 14.0, 19.0, 20.0, 26.0)).__next__
    pipeline = FacePipeline(
        detector=StubDetector(), aligner=StubAligner(), embedder=StubEmbedder(), clock_ms=clock
    )

    result = pipeline.process(make_image_bytes())

    assert result.timings == PipelineTimings(1.0, 2.0, 8.0, 10.0)
    assert len(result.faces) == 2


def test_pipeline_returns_faces_in_deterministic_spatial_order() -> None:
    pipeline = FacePipeline(detector=StubDetector(), aligner=StubAligner(), embedder=StubEmbedder())

    result = pipeline.process(make_image_bytes())

    assert result.image.width == 32
    assert result.image.height == 24
    assert [face.detection.bounding_box.x for face in result.faces] == [1, 10]
    assert all(isinstance(face, EmbeddedFace) for face in result.faces)
    np.testing.assert_allclose(result.faces[0].embedding, np.array([2**-0.5, 0.0, 2**-0.5]))
    assert result.faces[0].embedding.flags.writeable is False


def test_pipeline_rejects_embedding_dimension_mismatch() -> None:
    class WrongDimensionEmbedder(StubEmbedder):
        @property
        def dimension(self) -> int:
            return 2

    pipeline = FacePipeline(detector=StubDetector(), aligner=StubAligner(), embedder=WrongDimensionEmbedder())

    with pytest.raises(FaceInferenceError, match="embedding"):
        pipeline.process(make_image_bytes())
