from __future__ import annotations

import math
import time
from collections.abc import Callable
from dataclasses import dataclass

import numpy as np

from face_ai.domain import (
    EmbeddedFace,
    EmbeddingVector,
    FaceAligner,
    FaceDetector,
    FaceEmbedder,
    FaceInferenceError,
)
from face_ai.validation import ImageValidationLimits, ValidatedImage, validate_image


@dataclass(frozen=True, slots=True)
class PipelineTimings:
    decode_validation_ms: float
    detection_ms: float
    alignment_ms: float
    embedding_ms: float

    def __post_init__(self) -> None:
        values = (
            self.decode_validation_ms,
            self.detection_ms,
            self.alignment_ms,
            self.embedding_ms,
        )
        if any(not math.isfinite(value) or value < 0 for value in values):
            raise ValueError("pipeline timings must be finite and non-negative")


@dataclass(frozen=True, slots=True)
class PipelineResult:
    image: ValidatedImage
    faces: tuple[EmbeddedFace, ...]
    timings: PipelineTimings


def normalize_embedding(embedding: np.ndarray) -> EmbeddingVector:
    vector = np.asarray(embedding, dtype=np.float32)
    if vector.ndim != 1 or vector.size == 0:
        raise ValueError("embedding must be a non-empty one-dimensional vector")
    if not np.all(np.isfinite(vector)):
        raise ValueError("embedding must contain only finite values")

    norm = float(np.linalg.norm(vector))
    if not np.isfinite(norm) or norm <= np.finfo(np.float32).eps:
        raise ValueError("embedding norm must be greater than zero")

    normalized = np.asarray(vector / norm, dtype=np.float32)
    normalized.setflags(write=False)
    return normalized


class FacePipeline:
    def __init__(
        self,
        *,
        detector: FaceDetector,
        aligner: FaceAligner,
        embedder: FaceEmbedder,
        image_limits: ImageValidationLimits | None = None,
        clock_ms: Callable[[], float] | None = None,
    ) -> None:
        if embedder.dimension <= 0:
            raise ValueError("embedding dimension must be positive")
        self._detector = detector
        self._aligner = aligner
        self._embedder = embedder
        self._image_limits = image_limits
        self._clock_ms = clock_ms or (lambda: time.perf_counter_ns() / 1_000_000)

    def process(self, content: bytes) -> PipelineResult:
        started = self._clock_ms()
        image = validate_image(content, limits=self._image_limits)
        decode_validation_ms = self._clock_ms() - started

        started = self._clock_ms()
        try:
            detections = sorted(
                self._detector.detect(image.pixels),
                key=lambda face: (
                    face.bounding_box.y,
                    face.bounding_box.x,
                    face.bounding_box.height,
                    face.bounding_box.width,
                    -face.confidence,
                ),
            )
        except Exception as error:
            raise FaceInferenceError("detection") from error
        detection_ms = self._clock_ms() - started

        alignment_ms = 0.0
        embedding_ms = 0.0
        faces: list[EmbeddedFace] = []
        for detection in detections:
            started = self._clock_ms()
            try:
                aligned_face = self._aligner.align(image.pixels, detection)
            except Exception as error:
                raise FaceInferenceError("alignment") from error
            alignment_ms += self._clock_ms() - started

            started = self._clock_ms()
            try:
                embedding = normalize_embedding(self._embedder.embed(aligned_face))
            except Exception as error:
                raise FaceInferenceError("embedding") from error
            if embedding.size != self._embedder.dimension:
                raise FaceInferenceError("embedding")
            embedding_ms += self._clock_ms() - started
            faces.append(EmbeddedFace(detection=detection, embedding=embedding))

        timings = PipelineTimings(
            decode_validation_ms=decode_validation_ms,
            detection_ms=detection_ms,
            alignment_ms=alignment_ms,
            embedding_ms=embedding_ms,
        )
        return PipelineResult(image=image, faces=tuple(faces), timings=timings)
