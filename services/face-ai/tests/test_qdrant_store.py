from __future__ import annotations

from types import SimpleNamespace
from typing import Any

import numpy as np
import pytest
from face_ai.domain import SearchResult
from face_ai.qdrant_store import BenchmarkQdrantIndex
from face_ai.vector_store import VectorCollection, VectorDistance, VectorRecord
from qdrant_client.http import models


class FakeQdrantClient:
    def __init__(self, *, exists: bool = False, size: int = 3, distance: models.Distance = models.Distance.COSINE) -> None:
        self.exists = exists
        self.size = size
        self.distance = distance
        self.created: list[dict[str, Any]] = []
        self.payload_indexes: list[dict[str, Any]] = []
        self.upserts: list[dict[str, Any]] = []
        self.queries: list[dict[str, Any]] = []
        self.deleted: list[str] = []
        self.query_results: list[Any] = []

    def collection_exists(self, *, collection_name: str) -> bool:
        return self.exists

    def create_collection(self, **kwargs: Any) -> bool:
        self.created.append(kwargs)
        self.exists = True
        return True

    def get_collection(self, *, collection_name: str) -> Any:
        vectors = SimpleNamespace(size=self.size, distance=self.distance)
        return SimpleNamespace(config=SimpleNamespace(params=SimpleNamespace(vectors=vectors)))

    def create_payload_index(self, **kwargs: Any) -> None:
        self.payload_indexes.append(kwargs)

    def upsert(self, **kwargs: Any) -> None:
        self.upserts.append(kwargs)

    def query_points(self, **kwargs: Any) -> Any:
        self.queries.append(kwargs)
        return SimpleNamespace(points=self.query_results)

    def delete_collection(self, *, collection_name: str) -> None:
        self.deleted.append(collection_name)
        self.exists = False


def make_index(client: FakeQdrantClient) -> BenchmarkQdrantIndex:
    return BenchmarkQdrantIndex(
        client=client,
        collection=VectorCollection(
            name="benchmark-poc-v1",
            dimension=3,
            distance=VectorDistance.COSINE,
        ),
    )


def record(face_id: str, *, dataset_id: str = "dataset-a", event_id: str = "event-1") -> VectorRecord:
    return VectorRecord(
        face_id=face_id,
        photo_id=f"photo-{face_id}",
        dataset_id=dataset_id,
        event_id=event_id,
        embedding=np.asarray([1.0, 0.0, 0.0], dtype=np.float32),
    )


def test_create_configures_cosine_collection_and_scope_indexes() -> None:
    client = FakeQdrantClient()
    index = make_index(client)

    index.create()

    vector_config = client.created[0]["vectors_config"]
    assert vector_config.size == 3
    assert vector_config.distance == models.Distance.COSINE
    assert [call["field_name"] for call in client.payload_indexes] == ["dataset_id", "event_id"]
    assert all(call["field_schema"] == models.PayloadSchemaType.KEYWORD for call in client.payload_indexes)


def test_create_rejects_incompatible_existing_collection() -> None:
    client = FakeQdrantClient(exists=True, size=128)

    with pytest.raises(ValueError, match="dimension"):
        make_index(client).create()


def test_upsert_batches_vectors_and_stores_only_opaque_metadata() -> None:
    client = FakeQdrantClient()
    index = make_index(client)

    index.upsert([record("face-1"), record("face-2"), record("face-3")], batch_size=2)

    assert [len(call["points"]) for call in client.upserts] == [2, 1]
    point = client.upserts[0]["points"][0]
    assert point.vector == [1.0, 0.0, 0.0]
    assert point.payload == {
        "face_id": "face-1",
        "photo_id": "photo-face-1",
        "dataset_id": "dataset-a",
        "event_id": "event-1",
    }


def test_search_always_filters_by_dataset_and_event_without_returning_vectors() -> None:
    client = FakeQdrantClient()
    client.query_results = [
        SimpleNamespace(payload={"face_id": "face-1", "photo_id": "photo-1"}, score=0.9)
    ]
    index = make_index(client)

    results = index.search(
        np.asarray([1.0, 0.0, 0.0], dtype=np.float32),
        dataset_id="dataset-a",
        event_id="event-1",
        limit=5,
        score_threshold=0.75,
    )

    assert results == [SearchResult(face_id="face-1", photo_id="photo-1", score=0.9)]
    query = client.queries[0]
    conditions = query["query_filter"].must
    assert {(condition.key, condition.match.value) for condition in conditions} == {
        ("dataset_id", "dataset-a"),
        ("event_id", "event-1"),
    }
    assert query["with_payload"] == ["face_id", "photo_id"]
    assert query["with_vectors"] is False


@pytest.mark.parametrize("dataset_id,event_id", [("", "event-1"), ("dataset-a", "")])
def test_search_rejects_empty_scope(dataset_id: str, event_id: str) -> None:
    client = FakeQdrantClient()

    with pytest.raises(ValueError, match="partition"):
        make_index(client).search(
            np.asarray([1.0, 0.0, 0.0], dtype=np.float32),
            dataset_id=dataset_id,
            event_id=event_id,
            limit=5,
        )

    assert client.queries == []


def test_vector_dimension_is_checked_before_qdrant_call() -> None:
    client = FakeQdrantClient()

    with pytest.raises(ValueError, match="dimension"):
        make_index(client).upsert(
            [
                VectorRecord(
                    face_id="face-1",
                    photo_id="photo-1",
                    dataset_id="dataset-a",
                    event_id="event-1",
                    embedding=np.asarray([1.0, 0.0], dtype=np.float32),
                )
            ]
        )

    assert client.upserts == []


def test_teardown_is_idempotent() -> None:
    client = FakeQdrantClient(exists=True)
    index = make_index(client)

    index.teardown()
    index.teardown()

    assert client.deleted == ["benchmark-poc-v1"]
