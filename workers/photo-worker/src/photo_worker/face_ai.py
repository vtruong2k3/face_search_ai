from __future__ import annotations

from dataclasses import dataclass

import httpx

from photo_worker.errors import (
    FaceAIResponseError,
    FaceAIUnavailableError,
    TerminalProcessingError,
)


@dataclass(frozen=True, slots=True)
class BoundingBox:
    x: int
    y: int
    width: int
    height: int


@dataclass(frozen=True, slots=True)
class ExtractedFace:
    bounding_box: BoundingBox
    confidence: float
    embedding: list[float]


class FaceAIClient:
    def __init__(
        self,
        *,
        base_url: str,
        internal_token: str = "",
        timeout_s: float = 30.0,
        embedding_dimension: int = 512,
        http_client: httpx.AsyncClient | None = None,
    ) -> None:
        self._base_url = base_url.rstrip("/")
        self._internal_token = internal_token
        self._timeout_s = timeout_s
        self._embedding_dimension = embedding_dimension
        self._http_client = http_client
        self._owns_client = http_client is None

    async def aclose(self) -> None:
        if self._owns_client and self._http_client is not None:
            await self._http_client.aclose()

    async def extract_faces(self, image_bytes: bytes) -> list[ExtractedFace]:
        if not image_bytes:
            raise TerminalProcessingError("image content is empty")

        client = await self._client()
        headers = {"Content-Type": "application/octet-stream"}
        if self._internal_token:
            headers["X-Internal-Token"] = self._internal_token

        try:
            response = await client.post(
                f"{self._base_url}/internal/v1/extract-faces",
                content=image_bytes,
                headers=headers,
            )
        except httpx.HTTPError as error:
            raise FaceAIUnavailableError("face ai is unreachable") from error

        if response.status_code in (400, 413):
            raise TerminalProcessingError("face ai rejected the image")
        if response.status_code in (401, 403):
            raise TerminalProcessingError("face ai rejected the internal token")
        if response.status_code == 503 or response.status_code >= 500:
            raise FaceAIUnavailableError("face ai is unavailable")
        if response.status_code != 200:
            raise FaceAIUnavailableError(f"face ai returned {response.status_code}")

        try:
            payload = response.json()
        except ValueError as error:
            raise FaceAIResponseError("face ai returned invalid json") from error

        faces = payload.get("faces")
        if not isinstance(faces, list):
            raise FaceAIResponseError("face ai response is missing faces")

        extracted: list[ExtractedFace] = []
        for item in faces:
            if not isinstance(item, dict):
                raise FaceAIResponseError("face ai returned an invalid face")
            extracted.append(self._parse_face(item))
        return extracted

    async def _client(self) -> httpx.AsyncClient:
        if self._http_client is None:
            self._http_client = httpx.AsyncClient(timeout=self._timeout_s)
        return self._http_client

    def _parse_face(self, item: dict[object, object]) -> ExtractedFace:
        detection = item.get("detection")
        embedding = item.get("embedding")
        if not isinstance(detection, dict) or not isinstance(embedding, list):
            raise FaceAIResponseError("face ai returned an invalid face payload")
        if len(embedding) != self._embedding_dimension:
            raise FaceAIResponseError("face embedding dimension is invalid")
        if not all(isinstance(value, (int, float)) for value in embedding):
            raise FaceAIResponseError("face embedding contains non-numeric values")

        box = detection.get("bounding_box")
        confidence = detection.get("confidence")
        if not isinstance(box, dict) or not isinstance(confidence, (int, float)):
            raise FaceAIResponseError("face detection payload is invalid")
        try:
            parsed_box = BoundingBox(
                x=int(box["x"]),
                y=int(box["y"]),
                width=int(box["width"]),
                height=int(box["height"]),
            )
        except (KeyError, TypeError, ValueError) as error:
            raise FaceAIResponseError("face bounding box is invalid") from error
        if parsed_box.width <= 0 or parsed_box.height <= 0:
            raise FaceAIResponseError("face bounding box is invalid")
        return ExtractedFace(
            bounding_box=parsed_box,
            confidence=float(confidence),
            embedding=[float(value) for value in embedding],
        )
