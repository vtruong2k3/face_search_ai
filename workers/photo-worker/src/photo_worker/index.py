from __future__ import annotations

from collections.abc import Sequence
from dataclasses import dataclass
from typing import Any, Protocol
from uuid import UUID

from qdrant_client.http import models

from photo_worker.errors import TransientProcessingError


class QdrantClientPort(Protocol):
    def collection_exists(self, *, collection_name: str) -> bool: ...

    def create_collection(self, **kwargs: Any) -> Any: ...

    def create_payload_index(self, **kwargs: Any) -> Any: ...

    def upsert(self, **kwargs: Any) -> Any: ...

    def delete(self, **kwargs: Any) -> Any: ...


@dataclass(frozen=True, slots=True)
class IndexedFace:
    vector_point_id: UUID
    face_id: str
    organization_id: str
    event_id: str
    photo_id: str
    face_index: int
    embedding: list[float]


class FaceIndexError(TransientProcessingError):
    """Retryable Qdrant indexing failure."""


class FaceVectorIndex:
    def __init__(
        self,
        *,
        client: QdrantClientPort,
        collection_name: str,
        dimension: int,
    ) -> None:
        self._client: Any = client
        self._collection_name = collection_name
        self._dimension = dimension
        self._ready = False

    def ensure_collection(self) -> None:
        if self._ready:
            return
        try:
            if not self._client.collection_exists(collection_name=self._collection_name):
                self._client.create_collection(
                    collection_name=self._collection_name,
                    vectors_config=models.VectorParams(
                        size=self._dimension,
                        distance=models.Distance.COSINE,
                    ),
                )
                for field_name in ("organization_id", "event_id", "photo_id"):
                    self._client.create_payload_index(
                        collection_name=self._collection_name,
                        field_name=field_name,
                        field_schema=models.PayloadSchemaType.KEYWORD,
                        wait=True,
                    )
                self._client.create_payload_index(
                    collection_name=self._collection_name,
                    field_name="face_index",
                    field_schema=models.PayloadSchemaType.INTEGER,
                    wait=True,
                )
            self._ready = True
        except FaceIndexError:
            raise
        except Exception as error:
            raise FaceIndexError("failed to ensure Qdrant collection") from error

    def upsert_faces(self, faces: Sequence[IndexedFace]) -> None:
        self.ensure_collection()
        if not faces:
            return
        points = [self._point(face) for face in faces]
        try:
            self._client.upsert(
                collection_name=self._collection_name,
                points=points,
                wait=True,
            )
        except Exception as error:
            raise FaceIndexError("failed to upsert face vectors") from error

    def delete_extra_faces(
        self,
        *,
        organization_id: str,
        photo_id: str,
        keep_count: int,
    ) -> None:
        self.ensure_collection()
        try:
            self._client.delete(
                collection_name=self._collection_name,
                points_selector=models.FilterSelector(
                    filter=models.Filter(
                        must=[
                            models.FieldCondition(
                                key="organization_id",
                                match=models.MatchValue(value=organization_id),
                            ),
                            models.FieldCondition(
                                key="photo_id",
                                match=models.MatchValue(value=photo_id),
                            ),
                            models.FieldCondition(
                                key="face_index",
                                range=models.Range(gte=keep_count),
                            ),
                        ]
                    )
                ),
                wait=True,
            )
        except Exception as error:
            raise FaceIndexError("failed to delete extra face vectors") from error

    def _point(self, face: IndexedFace) -> models.PointStruct:
        if len(face.embedding) != self._dimension:
            raise FaceIndexError(f"embedding dimension must be {self._dimension}")
        return models.PointStruct(
            id=str(face.vector_point_id),
            vector=face.embedding,
            payload={
                "organization_id": face.organization_id,
                "event_id": face.event_id,
                "photo_id": face.photo_id,
                "face_id": face.face_id,
                "face_index": face.face_index,
            },
        )
