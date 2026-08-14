from __future__ import annotations

from collections.abc import Sequence
from dataclasses import dataclass
from enum import Enum
from typing import Protocol

import numpy as np

from face_ai.domain import EmbeddingVector, SearchResult


class VectorDistance(str, Enum):
    COSINE = "cosine"
    DOT = "dot"
    EUCLID = "euclid"


@dataclass(frozen=True, slots=True)
class VectorCollection:
    name: str
    dimension: int
    distance: VectorDistance

    def __post_init__(self) -> None:
        if not self.name.strip():
            raise ValueError("collection name must not be empty")
        if self.dimension <= 0:
            raise ValueError("vector dimension must be positive")


@dataclass(frozen=True, slots=True)
class VectorRecord:
    face_id: str
    photo_id: str
    dataset_id: str
    event_id: str
    embedding: EmbeddingVector

    def __post_init__(self) -> None:
        if not all(value.strip() for value in (self.face_id, self.photo_id, self.dataset_id, self.event_id)):
            raise ValueError("vector record identifiers must not be empty")
        vector = np.asarray(self.embedding)
        if vector.ndim != 1 or vector.size == 0:
            raise ValueError("embedding must be a non-empty one-dimensional vector")
        if not np.all(np.isfinite(vector)):
            raise ValueError("embedding must contain only finite values")


class VectorIndex(Protocol):
    def create(self) -> None: ...

    def upsert(self, records: Sequence[VectorRecord], *, batch_size: int = 100) -> None: ...

    def search(
        self,
        embedding: EmbeddingVector,
        *,
        dataset_id: str,
        event_id: str,
        limit: int,
        score_threshold: float | None = None,
    ) -> list[SearchResult]: ...

    def teardown(self) -> None: ...
