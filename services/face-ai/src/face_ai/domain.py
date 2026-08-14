from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol

import numpy as np
import numpy.typing as npt

ImageArray = npt.NDArray[np.uint8]
EmbeddingVector = npt.NDArray[np.float32]
Point = tuple[float, float]


class FaceInferenceError(RuntimeError):
    def __init__(self, stage: str) -> None:
        self.stage = stage
        super().__init__(f"face {stage} failed")


@dataclass(frozen=True, slots=True)
class BoundingBox:
    x: int
    y: int
    width: int
    height: int

    def __post_init__(self) -> None:
        if self.x < 0 or self.y < 0:
            raise ValueError("bounding box coordinates must be non-negative")
        if self.width <= 0 or self.height <= 0:
            raise ValueError("bounding box dimensions must be positive")


@dataclass(frozen=True, slots=True)
class FacialLandmarks:
    left_eye: Point
    right_eye: Point
    nose: Point
    left_mouth: Point
    right_mouth: Point


@dataclass(frozen=True, slots=True)
class DetectedFace:
    bounding_box: BoundingBox
    landmarks: FacialLandmarks
    confidence: float

    def __post_init__(self) -> None:
        if not 0.0 <= self.confidence <= 1.0:
            raise ValueError("detection confidence must be between zero and one")


@dataclass(frozen=True, slots=True)
class EmbeddedFace:
    detection: DetectedFace
    embedding: EmbeddingVector


@dataclass(frozen=True, slots=True)
class SearchResult:
    face_id: str
    photo_id: str
    score: float

    def __post_init__(self) -> None:
        if not self.face_id or not self.photo_id:
            raise ValueError("search result identifiers must not be empty")
        if not np.isfinite(self.score):
            raise ValueError("search result score must be finite")


class FaceDetector(Protocol):
    def detect(self, image: ImageArray) -> list[DetectedFace]: ...


class FaceAligner(Protocol):
    def align(self, image: ImageArray, face: DetectedFace) -> ImageArray: ...


class FaceEmbedder(Protocol):
    @property
    def dimension(self) -> int: ...

    def embed(self, aligned_face: ImageArray) -> EmbeddingVector: ...
