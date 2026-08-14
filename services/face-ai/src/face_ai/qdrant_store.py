from __future__ import annotations

from collections.abc import Sequence
from typing import Any, Protocol
from uuid import NAMESPACE_URL, uuid5

import numpy as np
from qdrant_client.http import models

from face_ai.domain import EmbeddingVector, SearchResult
from face_ai.vector_store import VectorCollection, VectorDistance, VectorRecord


class QdrantClientPort(Protocol):
    def collection_exists(self, *, collection_name: str) -> bool: ...

    def create_collection(self, **kwargs: Any) -> Any: ...

    def get_collection(self, *, collection_name: str) -> Any: ...

    def create_payload_index(self, **kwargs: Any) -> Any: ...

    def upsert(self, **kwargs: Any) -> Any: ...

    def query_points(self, **kwargs: Any) -> Any: ...

    def delete_collection(self, *, collection_name: str) -> Any: ...


_DISTANCE_MODELS = {
    VectorDistance.COSINE: models.Distance.COSINE,
    VectorDistance.DOT: models.Distance.DOT,
    VectorDistance.EUCLID: models.Distance.EUCLID,
}


class BenchmarkQdrantIndex:
    def __init__(self, *, client: QdrantClientPort, collection: VectorCollection) -> None:
        self._client = client
        self._collection = collection

    def create(self) -> None:
        if self._client.collection_exists(collection_name=self._collection.name):
            self._validate_existing_collection()
            return

        self._client.create_collection(
            collection_name=self._collection.name,
            vectors_config=models.VectorParams(
                size=self._collection.dimension,
                distance=_DISTANCE_MODELS[self._collection.distance],
            ),
        )
        for field_name in ("dataset_id", "event_id"):
            self._client.create_payload_index(
                collection_name=self._collection.name,
                field_name=field_name,
                field_schema=models.PayloadSchemaType.KEYWORD,
                wait=True,
            )

    def upsert(self, records: Sequence[VectorRecord], *, batch_size: int = 100) -> None:
        if batch_size <= 0:
            raise ValueError("batch size must be positive")

        points = [self._point(record) for record in records]
        for offset in range(0, len(points), batch_size):
            self._client.upsert(
                collection_name=self._collection.name,
                points=points[offset : offset + batch_size],
                wait=True,
            )

    def search(
        self,
        embedding: EmbeddingVector,
        *,
        dataset_id: str,
        event_id: str,
        limit: int,
        score_threshold: float | None = None,
    ) -> list[SearchResult]:
        if not dataset_id.strip() or not event_id.strip():
            raise ValueError("dataset and Event partition identifiers must not be empty")
        if limit <= 0:
            raise ValueError("search limit must be positive")
        vector = self._validate_vector(embedding)

        response = self._client.query_points(
            collection_name=self._collection.name,
            query=vector.tolist(),
            query_filter=models.Filter(
                must=[
                    models.FieldCondition(
                        key="dataset_id",
                        match=models.MatchValue(value=dataset_id),
                    ),
                    models.FieldCondition(
                        key="event_id",
                        match=models.MatchValue(value=event_id),
                    ),
                ]
            ),
            limit=limit,
            score_threshold=score_threshold,
            with_payload=["face_id", "photo_id"],
            with_vectors=False,
        )
        return [self._result(point) for point in response.points]

    def teardown(self) -> None:
        if self._client.collection_exists(collection_name=self._collection.name):
            self._client.delete_collection(collection_name=self._collection.name)

    def _point(self, record: VectorRecord) -> models.PointStruct:
        vector = self._validate_vector(record.embedding)
        return models.PointStruct(
            id=str(uuid5(NAMESPACE_URL, f"{self._collection.name}:{record.face_id}")),
            vector=vector.tolist(),
            payload={
                "face_id": record.face_id,
                "photo_id": record.photo_id,
                "dataset_id": record.dataset_id,
                "event_id": record.event_id,
            },
        )

    def _validate_vector(self, embedding: EmbeddingVector) -> np.ndarray:
        vector = np.asarray(embedding, dtype=np.float32)
        if vector.ndim != 1 or vector.size != self._collection.dimension:
            raise ValueError(f"vector dimension must be {self._collection.dimension}")
        if not np.all(np.isfinite(vector)):
            raise ValueError("vector must contain only finite values")
        return vector

    def _validate_existing_collection(self) -> None:
        collection = self._client.get_collection(collection_name=self._collection.name)
        vector_config = collection.config.params.vectors
        if not isinstance(vector_config, models.VectorParams) and (
            not hasattr(vector_config, "size") or not hasattr(vector_config, "distance")
        ):
            raise ValueError("named-vector collections are not supported")
        if vector_config.size != self._collection.dimension:
            raise ValueError("existing collection vector dimension does not match")
        if vector_config.distance != _DISTANCE_MODELS[self._collection.distance]:
            raise ValueError("existing collection distance metric does not match")

    @staticmethod
    def _result(point: Any) -> SearchResult:
        payload = point.payload or {}
        face_id = payload.get("face_id")
        photo_id = payload.get("photo_id")
        if not isinstance(face_id, str) or not isinstance(photo_id, str):
            raise TypeError("Qdrant result is missing required identifiers")
        return SearchResult(face_id=face_id, photo_id=photo_id, score=float(point.score))
