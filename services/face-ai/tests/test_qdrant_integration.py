from __future__ import annotations

import os
from uuid import uuid4

import numpy as np
import pytest
from face_ai.qdrant_store import BenchmarkQdrantIndex
from face_ai.vector_store import VectorCollection, VectorDistance, VectorRecord
from qdrant_client import QdrantClient

QDRANT_URL = os.getenv("FACE_AI_QDRANT_INTEGRATION_URL")
pytestmark = pytest.mark.skipif(
    not QDRANT_URL,
    reason="FACE_AI_QDRANT_INTEGRATION_URL is not configured",
)


def test_qdrant_search_isolated_by_dataset_and_event() -> None:
    assert QDRANT_URL is not None
    collection_name = f"benchmark-integration-{uuid4()}"
    client = QdrantClient(url=QDRANT_URL, timeout=10, check_compatibility=True)
    index = BenchmarkQdrantIndex(
        client=client,
        collection=VectorCollection(
            name=collection_name,
            dimension=3,
            distance=VectorDistance.COSINE,
        ),
    )

    try:
        index.create()
        index.upsert(
            [
                VectorRecord(
                    face_id="face-target",
                    photo_id="photo-target",
                    dataset_id="dataset-a",
                    event_id="event-1",
                    embedding=np.asarray([1.0, 0.0, 0.0], dtype=np.float32),
                ),
                VectorRecord(
                    face_id="face-other-event",
                    photo_id="photo-other-event",
                    dataset_id="dataset-a",
                    event_id="event-2",
                    embedding=np.asarray([1.0, 0.0, 0.0], dtype=np.float32),
                ),
                VectorRecord(
                    face_id="face-other-dataset",
                    photo_id="photo-other-dataset",
                    dataset_id="dataset-b",
                    event_id="event-1",
                    embedding=np.asarray([1.0, 0.0, 0.0], dtype=np.float32),
                ),
            ]
        )

        results = index.search(
            np.asarray([1.0, 0.0, 0.0], dtype=np.float32),
            dataset_id="dataset-a",
            event_id="event-1",
            limit=10,
        )

        assert [(result.face_id, result.photo_id) for result in results] == [
            ("face-target", "photo-target")
        ]
    finally:
        index.teardown()
        client.close()
