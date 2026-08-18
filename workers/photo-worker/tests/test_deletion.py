import json

import pytest
from photo_worker.deletion import DeletionProcessor
from photo_worker.jobs import (
    EVENT_DELETION,
    PHOTO_DELETION,
    EventDeletionPayload,
    JobEnvelope,
    PhotoDeletionPayload,
)
from photo_worker.settings import Settings
from photo_worker.storage import build_event_prefix, build_photo_prefix


def test_job_envelope_parses_photo_deletion() -> None:
    payload = {
        "photoId": "photo-1",
        "organizationId": "org-1",
        "eventId": "evt-1",
        "objectKey": "organizations/org-1/events/evt-1/photos/photo-1/original",
    }
    fields = {b"type": PHOTO_DELETION.encode(), b"payload": json.dumps(payload).encode()}
    job = JobEnvelope.from_stream(fields)
    assert job.type == PHOTO_DELETION
    assert isinstance(job.payload, PhotoDeletionPayload)
    assert job.payload.photo_id == "photo-1"
    assert job.payload.organization_id == "org-1"


def test_job_envelope_parses_event_deletion_ignoring_extra_fields() -> None:
    # The generic outbox publisher includes empty photo-processing fields; the
    # event-deletion payload must ignore them and bind the scope it needs.
    payload = {"organizationId": "org-1", "eventId": "evt-1", "photoId": "", "objectKey": ""}
    fields = {b"type": EVENT_DELETION.encode(), b"payload": json.dumps(payload).encode()}
    job = JobEnvelope.from_stream(fields)
    assert job.type == EVENT_DELETION
    assert isinstance(job.payload, EventDeletionPayload)
    assert job.payload.organization_id == "org-1"
    assert job.payload.event_id == "evt-1"


def test_prefixes_are_scoped_and_slash_terminated() -> None:
    assert build_photo_prefix("org-1", "evt-1", "photo-1") == "organizations/org-1/events/evt-1/photos/photo-1/"
    assert build_event_prefix("org-1", "evt-1") == "organizations/org-1/events/evt-1/"


def test_prefixes_reject_unsafe_segments() -> None:
    with pytest.raises(ValueError):
        build_photo_prefix("org/1", "evt-1", "photo-1")
    with pytest.raises(ValueError):
        build_event_prefix("org-1", "")


class FakeStorage:
    def __init__(self) -> None:
        self.removed_prefixes: list[tuple[str, str]] = []

    async def remove_prefix(self, bucket: str, prefix: str) -> int:
        self.removed_prefixes.append((bucket, prefix))
        return 3


class FakeIndex:
    def __init__(self) -> None:
        self.photo_calls: list[tuple[str, str]] = []
        self.event_calls: list[tuple[str, str]] = []

    def delete_photo_vectors(self, *, organization_id: str, photo_id: str) -> None:
        self.photo_calls.append((organization_id, photo_id))

    def delete_event_vectors(self, *, organization_id: str, event_id: str) -> None:
        self.event_calls.append((organization_id, event_id))


class FakePersist:
    def __init__(self) -> None:
        self.photo_calls: list[tuple[str, str]] = []
        self.event_calls: list[tuple[str, str]] = []

    def purge_photo_faces(self, *, organization_id: str, photo_id: str) -> None:
        self.photo_calls.append((organization_id, photo_id))

    def purge_event(self, *, organization_id: str, event_id: str) -> None:
        self.event_calls.append((organization_id, event_id))


def _processor() -> tuple[DeletionProcessor, FakeStorage, FakeIndex, FakePersist]:
    storage = FakeStorage()
    index = FakeIndex()
    persist = FakePersist()
    processor = DeletionProcessor(
        settings=Settings(minio_bucket="test-bucket"),
        storage=storage,  # type: ignore[arg-type]
        index=index,
        persist=persist,
    )
    return processor, storage, index, persist


@pytest.mark.asyncio
async def test_delete_photo_purges_objects_vectors_and_faces() -> None:
    processor, storage, index, persist = _processor()
    payload = PhotoDeletionPayload.model_validate(
        {"photoId": "photo-1", "organizationId": "org-1", "eventId": "evt-1"}
    )
    await processor.delete_photo(payload)
    assert storage.removed_prefixes == [("test-bucket", "organizations/org-1/events/evt-1/photos/photo-1/")]
    assert index.photo_calls == [("org-1", "photo-1")]
    assert persist.photo_calls == [("org-1", "photo-1")]


@pytest.mark.asyncio
async def test_delete_event_purges_objects_vectors_and_faces() -> None:
    processor, storage, index, persist = _processor()
    payload = EventDeletionPayload.model_validate({"organizationId": "org-1", "eventId": "evt-1"})
    await processor.delete_event(payload)
    assert storage.removed_prefixes == [("test-bucket", "organizations/org-1/events/evt-1/")]
    assert index.event_calls == [("org-1", "evt-1")]
    assert persist.event_calls == [("org-1", "evt-1")]


class _RecordingDeletion:
    def __init__(self) -> None:
        self.photos: list[str] = []
        self.events: list[str] = []

    async def delete_photo(self, payload: PhotoDeletionPayload) -> None:
        self.photos.append(payload.photo_id)

    async def delete_event(self, payload: EventDeletionPayload) -> None:
        self.events.append(payload.event_id)


@pytest.mark.asyncio
async def test_consumer_routes_deletion_jobs() -> None:
    from photo_worker.consumer import WorkerConsumer

    class FakeRedis:
        async def xgroup_create(self, *a: object, **k: object) -> bool:
            return True

    async def _noop(_job: JobEnvelope) -> None:
        return None

    # Provide a processor so the consumer does not build real Redis/DB/Qdrant
    # backends, then exercise default_process routing with a recording fake.
    consumer = WorkerConsumer(client=FakeRedis(), settings=Settings(), processor=_noop)  # type: ignore[arg-type]
    recording = _RecordingDeletion()
    consumer.deletion_processor = recording  # type: ignore[assignment]

    await consumer.default_process(
        JobEnvelope(type=PHOTO_DELETION, payload=PhotoDeletionPayload.model_validate(
            {"photoId": "photo-1", "organizationId": "org-1", "eventId": "evt-1"}))
    )
    await consumer.default_process(
        JobEnvelope(type=EVENT_DELETION, payload=EventDeletionPayload.model_validate(
            {"organizationId": "org-1", "eventId": "evt-1"}))
    )
    assert recording.photos == ["photo-1"]
    assert recording.events == ["evt-1"]


@pytest.mark.asyncio
async def test_delete_photo_is_idempotent_across_repeated_runs() -> None:
    processor, storage, index, persist = _processor()
    payload = PhotoDeletionPayload.model_validate(
        {"photoId": "photo-1", "organizationId": "org-1", "eventId": "evt-1"}
    )
    await processor.delete_photo(payload)
    await processor.delete_photo(payload)
    # Re-running is safe: each idempotent step simply runs again with no error.
    assert len(storage.removed_prefixes) == 2
    assert index.photo_calls == [("org-1", "photo-1"), ("org-1", "photo-1")]
    assert persist.photo_calls == [("org-1", "photo-1"), ("org-1", "photo-1")]
