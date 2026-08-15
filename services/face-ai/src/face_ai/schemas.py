from __future__ import annotations

from pydantic import BaseModel, Field


class BoundingBoxResponse(BaseModel):
    x: int = Field(..., description="X coordinate of the top-left corner")
    y: int = Field(..., description="Y coordinate of the top-left corner")
    width: int = Field(..., description="Width of the bounding box", gt=0)
    height: int = Field(..., description="Height of the bounding box", gt=0)


class FacialLandmarksResponse(BaseModel):
    left_eye: tuple[float, float]
    right_eye: tuple[float, float]
    nose: tuple[float, float]
    left_mouth: tuple[float, float]
    right_mouth: tuple[float, float]


class FaceDetectionResponse(BaseModel):
    bounding_box: BoundingBoxResponse
    landmarks: FacialLandmarksResponse
    confidence: float = Field(..., description="Detection confidence score between 0 and 1", ge=0.0, le=1.0)


class EmbeddedFaceResponse(BaseModel):
    detection: FaceDetectionResponse
    embedding: list[float] = Field(..., description="L2-normalized 512-dimensional feature vector")


class PipelineTimingsResponse(BaseModel):
    decode_validation_ms: float
    detection_ms: float
    alignment_ms: float
    embedding_ms: float


class ExtractFacesResponse(BaseModel):
    faces: list[EmbeddedFaceResponse]
    image_width: int
    image_height: int
    media_type: str
    timings: PipelineTimingsResponse


class ErrorResponse(BaseModel):
    error: str
    detail: str | None = None
