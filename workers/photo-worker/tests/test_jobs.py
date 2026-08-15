import json
import pytest
from photo_worker.jobs import JobEnvelope, PhotoProcessingPayload


def test_job_envelope_from_outbox_stream_format() -> None:
    payload_dict = {
        "photoId": "photo-123",
        "organizationId": "org-456",
        "eventId": "event-789",
        "objectKey": "organizations/org-456/events/event-789/photos/photo-123/original",
        "processingGeneration": 1,
        "idempotencyKey": "photo.process:photo-123:1",
        "attemptCount": 0,
    }
    fields = {
        b"type": b"photo.processing.requested",
        b"payload": json.dumps(payload_dict).encode("utf-8"),
    }
    job = JobEnvelope.from_stream(fields)
    assert job.type == "photo.processing.requested"
    assert isinstance(job.payload, PhotoProcessingPayload)
    assert job.payload.photo_id == "photo-123"
    assert job.payload.organization_id == "org-456"
    assert job.payload.event_id == "event-789"
    assert job.payload.object_key == payload_dict["objectKey"]
    assert job.payload.processing_generation == 1


def test_job_envelope_from_nested_job_format() -> None:
    raw_job = {
        "type": "photo.processing.requested",
        "payload": {
            "photoId": "photo-999",
            "organizationId": "org-999",
            "eventId": "event-999",
            "objectKey": "org-999/event-999/photo-999",
            "processingGeneration": 2,
        },
    }
    fields = {b"job": json.dumps(raw_job).encode("utf-8")}
    job = JobEnvelope.from_stream(fields)
    assert job.type == "photo.processing.requested"
    assert isinstance(job.payload, PhotoProcessingPayload)
    assert job.payload.photo_id == "photo-999"


def test_job_envelope_connectivity_smoke() -> None:
    fields = {
        b"type": b"connectivity.smoke",
        b"payload": b"{}",
    }
    job = JobEnvelope.from_stream(fields)
    assert job.type == "connectivity.smoke"
    assert job.payload == {}


def test_job_envelope_invalid_format() -> None:
    with pytest.raises(ValueError, match="unrecognized stream message format"):
        JobEnvelope.from_stream({b"unknown": b"123"})
