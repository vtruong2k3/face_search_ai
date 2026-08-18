"""Asynchronous purge of tombstoned photos and archived events.

The Go API tombstones the resource synchronously (photo -> 'deleted', event ->
'archived'), which makes it immediately non-searchable and non-downloadable, and
then writes a durable outbox message. This module consumes those messages and
removes the underlying data everywhere it lives:

* MinIO objects (original + every derivative) under the resource's key prefix.
* Qdrant face vectors filtered by the tenant + resource scope.
* PostgreSQL face rows (and, for an event, the photo tombstones).

Every step is tenant- and resource-scoped and idempotent, so a retried or
duplicated deletion message causes no error and no double effect. Steps run
storage -> vectors -> database so that a failure leaves the resource tombstoned
and retryable rather than leaving orphaned, searchable data.
"""

from __future__ import annotations

import asyncio
from typing import Protocol

import structlog

from photo_worker.jobs import EventDeletionPayload, PhotoDeletionPayload
from photo_worker.settings import Settings
from photo_worker.storage import (
    MinioStorageAdapter,
    build_event_prefix,
    build_photo_prefix,
)

log = structlog.get_logger()


class DeletionVectorIndex(Protocol):
    def delete_photo_vectors(self, *, organization_id: str, photo_id: str) -> None: ...

    def delete_event_vectors(self, *, organization_id: str, event_id: str) -> None: ...


class DeletionPersistence(Protocol):
    def purge_photo_faces(self, *, organization_id: str, photo_id: str) -> None: ...

    def purge_event(self, *, organization_id: str, event_id: str) -> None: ...


class DeletionProcessor:
    def __init__(
        self,
        *,
        settings: Settings,
        storage: MinioStorageAdapter,
        index: DeletionVectorIndex,
        persist: DeletionPersistence,
    ) -> None:
        self.settings = settings
        self.storage = storage
        self.index = index
        self.persist = persist

    async def delete_photo(self, payload: PhotoDeletionPayload) -> None:
        prefix = build_photo_prefix(
            payload.organization_id,
            payload.event_id,
            payload.photo_id,
        )
        removed = await self.storage.remove_prefix(self.settings.minio_bucket, prefix)
        await asyncio.to_thread(
            self.index.delete_photo_vectors,
            organization_id=payload.organization_id,
            photo_id=payload.photo_id,
        )
        await asyncio.to_thread(
            self.persist.purge_photo_faces,
            organization_id=payload.organization_id,
            photo_id=payload.photo_id,
        )
        log.info(
            "photo_purged",
            photo_id=payload.photo_id,
            objects_removed=removed,
        )

    async def delete_event(self, payload: EventDeletionPayload) -> None:
        prefix = build_event_prefix(payload.organization_id, payload.event_id)
        removed = await self.storage.remove_prefix(self.settings.minio_bucket, prefix)
        await asyncio.to_thread(
            self.index.delete_event_vectors,
            organization_id=payload.organization_id,
            event_id=payload.event_id,
        )
        await asyncio.to_thread(
            self.persist.purge_event,
            organization_id=payload.organization_id,
            event_id=payload.event_id,
        )
        log.info(
            "event_purged",
            event_id=payload.event_id,
            objects_removed=removed,
        )
