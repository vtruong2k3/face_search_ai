from __future__ import annotations

import time
from typing import Annotated

import structlog
from fastapi import APIRouter, Depends, HTTPException, Request, status

from face_ai.domain import FaceInferenceError
from face_ai.observability import record_inference
from face_ai.pipeline import FacePipeline
from face_ai.runtime import get_insightface_pipeline_runtime
from face_ai.schemas import (
    BoundingBoxResponse,
    EmbeddedFaceResponse,
    ErrorResponse,
    ExtractFacesResponse,
    FaceDetectionResponse,
    FacialLandmarksResponse,
    PipelineTimingsResponse,
)
from face_ai.security import verify_internal_token
from face_ai.settings import Settings, get_settings
from face_ai.validation import ImageValidationError

log = structlog.get_logger()

# Matches the default ImageValidationLimits.max_bytes; a Content-Length pre-check
# rejects oversized payloads before they are buffered into memory.
_MAX_REQUEST_BYTES = 20 * 1024 * 1024

router = APIRouter(
    prefix="/internal",
    tags=["internal"],
    dependencies=[Depends(verify_internal_token)],
    responses={
        401: {"model": ErrorResponse, "description": "Unauthorized - Missing token"},
        403: {"model": ErrorResponse, "description": "Forbidden - Invalid token"},
        413: {"model": ErrorResponse, "description": "Payload Too Large"},
        500: {"model": ErrorResponse, "description": "Internal Server Error"},
        503: {"model": ErrorResponse, "description": "Service Unavailable - Pipeline not ready"},
    },
)


def get_pipeline(settings: Annotated[Settings, Depends(get_settings)]) -> FacePipeline | None:
    return get_insightface_pipeline_runtime(settings).pipeline()


@router.post(
    "/v1/extract-faces",
    response_model=ExtractFacesResponse,
    summary="Extract faces and embeddings from an image",
    description="Internal endpoint to detect faces, align landmarks, and compute embeddings.",
)
async def extract_faces(
    request: Request,
    pipeline: Annotated[FacePipeline | None, Depends(get_pipeline)],
) -> ExtractFacesResponse:
    # Record a bounded outcome class and the request latency without ever observing
    # the image bytes, the embedding, or any identifier. The outcome is set before
    # each early return/raise and emitted once in the finally block.
    started = time.perf_counter()
    outcome = "error"
    try:
        if pipeline is None:
            outcome = "not_ready"
            raise HTTPException(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                detail="inference_pipeline_not_ready",
            )

        content_length = request.headers.get("content-length")
        if content_length and content_length.isdigit() and int(content_length) > _MAX_REQUEST_BYTES:
            outcome = "payload_too_large"
            raise HTTPException(
                status_code=status.HTTP_413_CONTENT_TOO_LARGE,
                detail="image exceeds the byte limit",
            )

        image_bytes = await request.body()
        if not image_bytes:
            outcome = "empty_image"
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="image content is empty",
            )

        try:
            response = _process_image(pipeline, image_bytes)
        except ImageValidationError as error:
            # Map validation errors to HTTP status codes without leaking internals.
            error_msg = str(error)
            if "byte limit" in error_msg.lower():
                outcome = "payload_too_large"
                raise HTTPException(
                    status_code=status.HTTP_413_CONTENT_TOO_LARGE,
                    detail=error_msg,
                )
            outcome = "validation_error"
            raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=error_msg)
        except FaceInferenceError as error:
            outcome = "inference_error"
            log.error("face_inference_failed", stage=error.stage)
            # Sanitize the error message to avoid leaking internals.
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="face inference failed",
            )
        outcome = "ok"
        return response
    finally:
        record_inference(outcome, time.perf_counter() - started)


def _process_image(pipeline: FacePipeline, image_bytes: bytes) -> ExtractFacesResponse:
    # Pipeline execution blocks the event loop since it is CPU bound.
    # This is expected for now as FastAPI standard workers process sequentially.
    # Could be pushed to a ThreadPoolExecutor in the future if concurrency is needed.
    result = pipeline.process(image_bytes)

    faces_response = [
        EmbeddedFaceResponse(
            detection=FaceDetectionResponse(
                bounding_box=BoundingBoxResponse(
                    x=face.detection.bounding_box.x,
                    y=face.detection.bounding_box.y,
                    width=face.detection.bounding_box.width,
                    height=face.detection.bounding_box.height,
                ),
                landmarks=FacialLandmarksResponse(
                    left_eye=face.detection.landmarks.left_eye,
                    right_eye=face.detection.landmarks.right_eye,
                    nose=face.detection.landmarks.nose,
                    left_mouth=face.detection.landmarks.left_mouth,
                    right_mouth=face.detection.landmarks.right_mouth,
                ),
                confidence=face.detection.confidence,
            ),
            embedding=face.embedding.tolist(),
        )
        for face in result.faces
    ]

    timings = PipelineTimingsResponse(
        decode_validation_ms=result.timings.decode_validation_ms,
        detection_ms=result.timings.detection_ms,
        alignment_ms=result.timings.alignment_ms,
        embedding_ms=result.timings.embedding_ms,
    )

    response = ExtractFacesResponse(
        faces=faces_response,
        image_width=result.image.width,
        image_height=result.image.height,
        media_type=result.image.media_type,
        timings=timings,
    )

    log.info(
        "extract_faces_completed",
        face_count=len(result.faces),
        width=result.image.width,
        height=result.image.height,
        media_type=result.image.media_type,
        detection_ms=round(result.timings.detection_ms, 2),
        embedding_ms=round(result.timings.embedding_ms, 2),
    )
    return response
