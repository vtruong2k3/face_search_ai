import json
from typing import Any

from pydantic import BaseModel


class JobEnvelope(BaseModel):
    version: int = 1
    job_id: str
    type: str
    payload: dict[str, Any] = {}

    @classmethod
    def from_stream(cls, fields: dict[bytes, bytes]) -> "JobEnvelope":
        raw = fields.get(b"job")
        if raw is None:
            raise ValueError("missing job field")
        return cls.model_validate_json(raw)

    def to_json(self) -> str:
        return json.dumps(self.model_dump(), separators=(",", ":"))
