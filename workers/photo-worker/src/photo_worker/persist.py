from __future__ import annotations

import json
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Any, Protocol
from uuid import UUID

from psycopg_pool import ConnectionPool

from photo_worker.errors import TransientProcessingError
from photo_worker.face_ai import BoundingBox


class PersistenceError(TransientProcessingError):
    """Retryable PostgreSQL persistence failure."""


@dataclass(frozen=True, slots=True)
class PersistedFace:
    face_id: str
    vector_point_id: UUID
    organization_id: str
    event_id: str
    photo_id: str
    face_index: int
    bounding_box: BoundingBox


@dataclass(frozen=True, slots=True)
class ClaimResult:
    claimed: bool
    already_ready: bool
    skip: bool


class PhotoPersistence(Protocol):
    def delete_faces(self, *, organization_id: str, photo_id: str, keep_count: int) -> None: ...

    def claim(self, *, organization_id: str, photo_id: str, processing_generation: int) -> ClaimResult: ...

    def replace_faces(self, faces: Sequence[PersistedFace], *, keep_count: int) -> None: ...

    def mark_ready(self, *, organization_id: str, photo_id: str, processing_generation: int) -> None: ...

    def mark_failed(
        self,
        *,
        organization_id: str,
        photo_id: str,
        processing_generation: int,
        failure_code: str,
    ) -> None: ...


class PostgresPhotoPersistence:
    def __init__(self, pool: ConnectionPool[Any]) -> None:
        self._pool = pool

    def claim(self, *, organization_id: str, photo_id: str, processing_generation: int) -> ClaimResult:
        try:
            with self._pool.connection() as connection:  # type: ignore[union-attr]
                with connection.transaction():
                    row = connection.execute(
                        """
                        UPDATE photos
                        SET status = 'processing', failure_code = NULL, updated_at = now()
                        WHERE organization_id = %s
                          AND id = %s
                          AND processing_generation = %s
                          AND status IN ('queued', 'processing')
                        RETURNING id
                        """,
                        (organization_id, photo_id, processing_generation),
                    ).fetchone()
                    if row is not None:
                        return ClaimResult(claimed=True, already_ready=False, skip=False)

                    current = connection.execute(
                        """
                        SELECT status, processing_generation
                        FROM photos
                        WHERE organization_id = %s AND id = %s
                        """,
                        (organization_id, photo_id),
                    ).fetchone()
        except Exception as error:
            raise PersistenceError("failed to claim photo") from error

        if current is None:
            raise PersistenceError("photo not found")
        status, generation = current[0], int(current[1])
        if generation != processing_generation:
            return ClaimResult(claimed=False, already_ready=False, skip=True)
        if status == "ready":
            return ClaimResult(claimed=False, already_ready=True, skip=False)
        if status == "deleted":
            return ClaimResult(claimed=False, already_ready=False, skip=True)
        raise PersistenceError(f"photo is not claimable in status {status}")

    def replace_faces(self, faces: Sequence[PersistedFace], *, keep_count: int) -> None:
        if keep_count < 0:
            raise PersistenceError("keep_count must not be negative")
        try:
            with self._pool.connection() as connection:  # type: ignore[union-attr]
                with connection.transaction():
                    if faces:
                        organization_id = faces[0].organization_id
                        photo_id = faces[0].photo_id
                        for face in faces:
                            box = {
                                "x": face.bounding_box.x,
                                "y": face.bounding_box.y,
                                "width": face.bounding_box.width,
                                "height": face.bounding_box.height,
                            }
                            connection.execute(
                                """
                                INSERT INTO faces (
                                    id, organization_id, event_id, photo_id,
                                    face_index, vector_point_id, bounding_box
                                ) VALUES (%s, %s, %s, %s, %s, %s, %s::jsonb)
                                ON CONFLICT (organization_id, photo_id, face_index) DO UPDATE
                                SET event_id = EXCLUDED.event_id,
                                    vector_point_id = EXCLUDED.vector_point_id,
                                    bounding_box = EXCLUDED.bounding_box
                                """,
                                (
                                    face.face_id,
                                    face.organization_id,
                                    face.event_id,
                                    face.photo_id,
                                    face.face_index,
                                    str(face.vector_point_id),
                                    json.dumps(box),
                                ),
                            )
                    else:
                        organization_id = ""
                        photo_id = ""

                    if faces:
                        connection.execute(
                            """
                            DELETE FROM faces
                            WHERE organization_id = %s AND photo_id = %s AND face_index >= %s
                            """,
                            (organization_id, photo_id, keep_count),
                        )
        except PersistenceError:
            raise
        except Exception as error:
            raise PersistenceError("failed to persist faces") from error

    def delete_faces(self, *, organization_id: str, photo_id: str, keep_count: int) -> None:
        try:
            with self._pool.connection() as connection:  # type: ignore[union-attr]
                with connection.transaction():
                    connection.execute(
                        """
                        DELETE FROM faces
                        WHERE organization_id = %s AND photo_id = %s AND face_index >= %s
                        """,
                        (organization_id, photo_id, keep_count),
                    )
        except Exception as error:
            raise PersistenceError("failed to delete extra faces") from error

    def purge_photo_faces(self, *, organization_id: str, photo_id: str) -> None:
        """Delete every face row for a single photo. Always scoped by both
        organization and photo, so it can never touch another tenant. Idempotent:
        deleting already-absent rows affects nothing."""
        if not organization_id or not photo_id:
            raise PersistenceError("organization_id and photo_id are required")
        try:
            with self._pool.connection() as connection:  # type: ignore[union-attr]
                with connection.transaction():
                    connection.execute(
                        """
                        DELETE FROM faces
                        WHERE organization_id = %s AND photo_id = %s
                        """,
                        (organization_id, photo_id),
                    )
        except Exception as error:
            raise PersistenceError("failed to purge photo faces") from error

    def purge_event(self, *, organization_id: str, event_id: str) -> None:
        """Purge an entire event's face rows and tombstone all of its photos.
        Always scoped by both organization and event. Idempotent: re-running
        deletes nothing further and re-affirms the photo tombstones."""
        if not organization_id or not event_id:
            raise PersistenceError("organization_id and event_id are required")
        try:
            with self._pool.connection() as connection:  # type: ignore[union-attr]
                with connection.transaction():
                    connection.execute(
                        """
                        DELETE FROM faces
                        WHERE organization_id = %s AND event_id = %s
                        """,
                        (organization_id, event_id),
                    )
                    connection.execute(
                        """
                        UPDATE photos
                        SET status = 'deleted', failure_code = NULL, updated_at = now()
                        WHERE organization_id = %s AND event_id = %s AND status <> 'deleted'
                        """,
                        (organization_id, event_id),
                    )
        except Exception as error:
            raise PersistenceError("failed to purge event") from error

    def mark_ready(self, *, organization_id: str, photo_id: str, processing_generation: int) -> None:
        try:
            with self._pool.connection() as connection:  # type: ignore[union-attr]
                with connection.transaction():
                    row = connection.execute(
                        """
                        UPDATE photos
                        SET status = 'ready', failure_code = NULL, updated_at = now()
                        WHERE organization_id = %s
                          AND id = %s
                          AND processing_generation = %s
                          AND status = 'processing'
                        RETURNING id
                        """,
                        (organization_id, photo_id, processing_generation),
                    ).fetchone()
        except Exception as error:
            raise PersistenceError("failed to mark photo ready") from error
        if row is None:
            raise PersistenceError("photo could not be marked ready")

    def mark_failed(
        self,
        *,
        organization_id: str,
        photo_id: str,
        processing_generation: int,
        failure_code: str,
    ) -> None:
        try:
            with self._pool.connection() as connection:  # type: ignore[union-attr]
                with connection.transaction():
                    connection.execute(
                        """
                        UPDATE photos
                        SET status = 'failed', failure_code = %s, updated_at = now()
                        WHERE organization_id = %s
                          AND id = %s
                          AND processing_generation = %s
                          AND status IN ('queued', 'processing')
                        """,
                        (failure_code[:64], organization_id, photo_id, processing_generation),
                    )
        except Exception as error:
            raise PersistenceError("failed to mark photo failed") from error
