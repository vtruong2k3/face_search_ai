from __future__ import annotations

import asyncio
from collections.abc import Sequence
from typing import Protocol

import structlog

from photo_worker.errors import PhotoWorkerError, TerminalProcessingError
from photo_worker.face_ai import ExtractedFace
from photo_worker.ids import face_id, vector_point_id
from photo_worker.image import ImageProcessor
from photo_worker.index import IndexedFace
from photo_worker.jobs import PhotoProcessingPayload
from photo_worker.persist import PersistedFace, PhotoPersistence
from photo_worker.settings import Settings
from photo_worker.storage import MinioStorageAdapter, build_object_key

log = structlog.get_logger()


class FaceExtractor(Protocol):
    async def extract_faces(self, image_bytes: bytes) -> list[ExtractedFace]: ...


class VectorIndex(Protocol):
    def upsert_faces(self, faces: Sequence[IndexedFace]) -> None: ...

    def delete_extra_faces(self, *, organization_id: str, photo_id: str, keep_count: int) -> None: ...


class PhotoProcessor:
    def __init__(
        self,
        *,
        settings: Settings,
        storage: MinioStorageAdapter,
        image_processor: ImageProcessor,
        face_ai: FaceExtractor,
        index: VectorIndex,
        persist: PhotoPersistence,
    ) -> None:
        self.settings = settings
        self.storage = storage
        self.image_processor = image_processor
        self.face_ai = face_ai
        self.index = index
        self.persist = persist

    async def process(self, payload: PhotoProcessingPayload) -> None:
        claim = await asyncio.to_thread(
            self.persist.claim,
            organization_id=payload.organization_id,
            photo_id=payload.photo_id,
            processing_generation=payload.processing_generation,
        )
        if claim.skip or claim.already_ready:
            log.info(
                "photo_processing_skipped",
                photo_id=payload.photo_id,
                already_ready=claim.already_ready,
                skip=claim.skip,
            )
            return

        original_bytes = await self.storage.get_object(
            bucket=self.settings.minio_bucket,
            object_key=payload.object_key,
        )

        try:
            derivatives = await asyncio.to_thread(self.image_processor.generate_derivatives, original_bytes)
        except PhotoWorkerError as error:
            raise TerminalProcessingError(str(error)) from error

        for variant, result in derivatives.items():
            derivative_key = build_object_key(
                organization_id=payload.organization_id,
                event_id=payload.event_id,
                photo_id=payload.photo_id,
                variant=variant.value,
            )
            await self.storage.put_object(
                bucket=self.settings.minio_bucket,
                object_key=derivative_key,
                data=result.data,
                content_type=result.media_type,
            )

        extracted = await self.face_ai.extract_faces(original_bytes)
        persisted, indexed = self._records(payload, extracted)

        await asyncio.to_thread(self.index.upsert_faces, indexed)
        await asyncio.to_thread(
            self.index.delete_extra_faces,
            organization_id=payload.organization_id,
            photo_id=payload.photo_id,
            keep_count=len(extracted),
        )

        if persisted:
            await asyncio.to_thread(self.persist.replace_faces, persisted, keep_count=len(extracted))
        else:
            await asyncio.to_thread(
                self.persist.delete_faces,
                organization_id=payload.organization_id,
                photo_id=payload.photo_id,
                keep_count=0,
            )

        await asyncio.to_thread(
            self.persist.mark_ready,
            organization_id=payload.organization_id,
            photo_id=payload.photo_id,
            processing_generation=payload.processing_generation,
        )
        log.info(
            "photo_indexed",
            photo_id=payload.photo_id,
            face_count=len(extracted),
            processing_generation=payload.processing_generation,
        )

    def fail_photo(self, payload: PhotoProcessingPayload, failure_code: str) -> None:
        self.persist.mark_failed(
            organization_id=payload.organization_id,
            photo_id=payload.photo_id,
            processing_generation=payload.processing_generation,
            failure_code=failure_code,
        )

    def _records(
        self,
        payload: PhotoProcessingPayload,
        extracted: Sequence[ExtractedFace],
    ) -> tuple[list[PersistedFace], list[IndexedFace]]:
        persisted: list[PersistedFace] = []
        indexed: list[IndexedFace] = []
        for index, face in enumerate(extracted):
            logical_id = face_id(payload.organization_id, payload.photo_id, index)
            point_id = vector_point_id(
                self.settings.qdrant_collection,
                payload.organization_id,
                payload.photo_id,
                index,
            )
            persisted.append(
                PersistedFace(
                    face_id=logical_id,
                    vector_point_id=point_id,
                    organization_id=payload.organization_id,
                    event_id=payload.event_id,
                    photo_id=payload.photo_id,
                    face_index=index,
                    bounding_box=face.bounding_box,
                )
            )
            indexed.append(
                IndexedFace(
                    vector_point_id=point_id,
                    face_id=logical_id,
                    organization_id=payload.organization_id,
                    event_id=payload.event_id,
                    photo_id=payload.photo_id,
                    face_index=index,
                    embedding=face.embedding,
                )
            )
        return persisted, indexed
