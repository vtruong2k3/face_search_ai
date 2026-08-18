import json
from typing import Any

from pydantic import BaseModel, Field

PHOTO_PROCESSING = "photo.processing.requested"
PHOTO_DELETION = "photo.deletion.requested"
EVENT_DELETION = "event.deletion.requested"


class PhotoProcessingPayload(BaseModel):
    photo_id: str = Field(alias="photoId")
    organization_id: str = Field(alias="organizationId")
    event_id: str = Field(alias="eventId")
    object_key: str = Field(alias="objectKey")
    processing_generation: int = Field(default=0, alias="processingGeneration")
    idempotency_key: str = Field(default="", alias="idempotencyKey")
    attempt_count: int = Field(default=0, alias="attemptCount")


class PhotoDeletionPayload(BaseModel):
    """Purge instruction for a single tombstoned photo. The API has already set
    the photo to 'deleted'; this drives removal of its stored objects, vectors,
    and face rows."""

    photo_id: str = Field(alias="photoId")
    organization_id: str = Field(alias="organizationId")
    event_id: str = Field(alias="eventId")
    object_key: str = Field(default="", alias="objectKey")


class EventDeletionPayload(BaseModel):
    """Purge instruction for an archived (deleted) event. Drives removal of every
    photo's stored objects, vectors, and face rows for the event and tombstones
    its photos."""

    organization_id: str = Field(alias="organizationId")
    event_id: str = Field(alias="eventId")


class JobEnvelope(BaseModel):
    type: str
    payload: PhotoProcessingPayload | PhotoDeletionPayload | EventDeletionPayload | dict[str, Any]

    @classmethod
    def from_stream(cls, fields: dict[Any, Any]) -> "JobEnvelope":
        # Normalize fields to string keys
        norm: dict[str, Any] = {}
        for k, v in fields.items():
            k_str = k.decode("utf-8") if isinstance(k, bytes) else str(k)
            v_val = v.decode("utf-8") if isinstance(v, bytes) else v
            norm[k_str] = v_val

        # Case 1: Outbox publisher format: fields {"type": "photo.processing.requested", "payload": "{...}"}
        if "type" in norm and "payload" in norm:
            raw_payload = norm["payload"]
            if isinstance(raw_payload, str):
                parsed = json.loads(raw_payload)
            else:
                parsed = raw_payload

            if isinstance(parsed, dict):
                return cls(type=norm["type"], payload=_parse_payload(norm["type"], parsed))
            return cls(type=norm["type"], payload={})

        # Case 2: Legacy or nested format {"job": "{...}"}
        if "job" in norm:
            raw_job = norm["job"]
            parsed_job = json.loads(raw_job) if isinstance(raw_job, str) else raw_job
            job_type = parsed_job.get("type", "unknown")
            payload_data = parsed_job.get("payload", {})
            if isinstance(payload_data, dict):
                return cls(type=job_type, payload=_parse_payload(job_type, payload_data))
            return cls(type=job_type, payload=payload_data)

        raise ValueError(f"unrecognized stream message format: {norm}")


def _parse_payload(job_type: str, data: dict[str, Any]) -> Any:
    """Bind a raw payload dict to its typed model based on the job type. Unknown
    types keep the raw dict so the consumer can reject them explicitly."""
    if job_type == PHOTO_PROCESSING:
        return PhotoProcessingPayload.model_validate(data)
    if job_type == PHOTO_DELETION:
        return PhotoDeletionPayload.model_validate(data)
    if job_type == EVENT_DELETION:
        return EventDeletionPayload.model_validate(data)
    return data
