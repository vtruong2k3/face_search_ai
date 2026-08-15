import json
from typing import Any
from pydantic import BaseModel, Field


class PhotoProcessingPayload(BaseModel):
    photo_id: str = Field(alias="photoId")
    organization_id: str = Field(alias="organizationId")
    event_id: str = Field(alias="eventId")
    object_key: str = Field(alias="objectKey")
    processing_generation: int = Field(default=0, alias="processingGeneration")
    idempotency_key: str = Field(default="", alias="idempotencyKey")
    attempt_count: int = Field(default=0, alias="attemptCount")


class JobEnvelope(BaseModel):
    type: str
    payload: PhotoProcessingPayload | dict[str, Any]

    @classmethod
    def from_stream(cls, fields: dict[bytes | str, bytes | str]) -> "JobEnvelope":
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

            if norm["type"] == "photo.processing.requested" and isinstance(parsed, dict):
                return cls(type=norm["type"], payload=PhotoProcessingPayload.model_validate(parsed))
            return cls(type=norm["type"], payload=parsed if isinstance(parsed, dict) else {})

        # Case 2: Legacy or nested format {"job": "{...}"}
        if "job" in norm:
            raw_job = norm["job"]
            parsed_job = json.loads(raw_job) if isinstance(raw_job, str) else raw_job
            job_type = parsed_job.get("type", "unknown")
            payload_data = parsed_job.get("payload", {})
            if job_type == "photo.processing.requested" and isinstance(payload_data, dict):
                return cls(type=job_type, payload=PhotoProcessingPayload.model_validate(payload_data))
            return cls(type=job_type, payload=payload_data)

        raise ValueError(f"unrecognized stream message format: {norm}")
