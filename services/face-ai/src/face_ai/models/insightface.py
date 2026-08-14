from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Protocol

import numpy as np

from face_ai.domain import (
    BoundingBox,
    DetectedFace,
    EmbeddingVector,
    FacialLandmarks,
    ImageArray,
)


class FaceAnalysisPort(Protocol):
    models: dict[str, Any]

    def prepare(self, **kwargs: Any) -> None: ...

    def get(self, image: np.ndarray) -> list[Any]: ...


class RecognitionPort(Protocol):
    def get_feat(self, image: np.ndarray) -> np.ndarray: ...


AnalysisFactory = Callable[..., FaceAnalysisPort]
AlignmentFunction = Callable[[np.ndarray, np.ndarray, int], np.ndarray]


@dataclass(frozen=True, slots=True)
class InsightFaceAdapters:
    detector: InsightFaceDetector
    aligner: InsightFaceAligner
    embedder: InsightFaceEmbedder


class InsightFaceDetector:
    def __init__(self, analysis: FaceAnalysisPort) -> None:
        self._analysis = analysis

    def detect(self, image: ImageArray) -> list[DetectedFace]:
        height, width = image.shape[:2]
        bgr = np.ascontiguousarray(image[:, :, ::-1])
        faces = self._analysis.get(bgr)
        return [self._map_face(face, width=width, height=height) for face in faces]

    @staticmethod
    def _map_face(face: Any, *, width: int, height: int) -> DetectedFace:
        bbox = np.asarray(face.bbox, dtype=np.float32)
        landmarks = np.asarray(face.kps, dtype=np.float32)
        confidence = float(face.det_score)
        if bbox.shape != (4,) or not np.all(np.isfinite(bbox)):
            raise ValueError("detector returned an invalid bounding box")
        if landmarks.shape != (5, 2) or not np.all(np.isfinite(landmarks)):
            raise ValueError("detector returned invalid five-point landmarks")
        if not np.isfinite(confidence):
            raise ValueError("detector returned an invalid confidence")

        x1 = max(0, min(width, int(np.floor(bbox[0]))))
        y1 = max(0, min(height, int(np.floor(bbox[1]))))
        x2 = max(0, min(width, int(np.ceil(bbox[2]))))
        y2 = max(0, min(height, int(np.ceil(bbox[3]))))
        if x2 <= x1 or y2 <= y1:
            raise ValueError("detector returned an empty bounding box")

        points = [
            (float(point[0]), float(point[1]))
            for point in landmarks
        ]
        return DetectedFace(
            bounding_box=BoundingBox(x=x1, y=y1, width=x2 - x1, height=y2 - y1),
            landmarks=FacialLandmarks(
                left_eye=points[0],
                right_eye=points[1],
                nose=points[2],
                left_mouth=points[3],
                right_mouth=points[4],
            ),
            confidence=confidence,
        )


class InsightFaceAligner:
    def __init__(self, align: AlignmentFunction) -> None:
        self._align = align

    def align(self, image: ImageArray, face: DetectedFace) -> ImageArray:
        landmarks = np.asarray(
            [
                face.landmarks.left_eye,
                face.landmarks.right_eye,
                face.landmarks.nose,
                face.landmarks.left_mouth,
                face.landmarks.right_mouth,
            ],
            dtype=np.float32,
        )
        bgr = np.ascontiguousarray(image[:, :, ::-1])
        crop = np.asarray(self._align(bgr, landmarks, 112), dtype=np.uint8)
        if crop.shape != (112, 112, 3):
            raise ValueError("alignment must return a 112 by 112 color crop")
        rgb = np.ascontiguousarray(crop[:, :, ::-1])
        rgb.setflags(write=False)
        return rgb


class InsightFaceEmbedder:
    dimension = 512

    def __init__(self, recognition: RecognitionPort) -> None:
        self._recognition = recognition

    def embed(self, aligned_face: ImageArray) -> EmbeddingVector:
        bgr = np.ascontiguousarray(aligned_face[:, :, ::-1])
        output = np.asarray(self._recognition.get_feat(bgr), dtype=np.float32).reshape(-1)
        if output.size != self.dimension:
            raise ValueError("recognition model must return one 512-dimensional embedding")
        if not np.all(np.isfinite(output)):
            raise ValueError("recognition model embedding must contain only finite values")
        return output


def prepare_buffalo_l(
    *,
    model_root: Path,
    analysis_factory: AnalysisFactory | None = None,
    align: AlignmentFunction | None = None,
    detection_size: tuple[int, int] = (640, 640),
    detection_threshold: float = 0.5,
) -> InsightFaceAdapters:
    pack_path = model_root / "models" / "buffalo_l"
    if not pack_path.is_dir():
        raise RuntimeError("buffalo_l model pack is not available locally")

    if analysis_factory is None or align is None:
        from insightface.app import FaceAnalysis  # type: ignore[import-not-found]
        from insightface.utils.face_align import (  # type: ignore[import-not-found]
            norm_crop,
        )

        analysis_factory = analysis_factory or FaceAnalysis
        align = align or (lambda image, landmarks, size: norm_crop(image, landmarks, image_size=size))

    analysis = analysis_factory(
        name="buffalo_l",
        root=str(model_root),
        providers=["CPUExecutionProvider"],
    )
    analysis.prepare(
        ctx_id=-1,
        det_size=detection_size,
        det_thresh=detection_threshold,
    )
    recognition = analysis.models.get("recognition")
    if recognition is None:
        raise RuntimeError("buffalo_l recognition model is unavailable")

    return InsightFaceAdapters(
        detector=InsightFaceDetector(analysis),
        aligner=InsightFaceAligner(align),
        embedder=InsightFaceEmbedder(recognition),
    )
