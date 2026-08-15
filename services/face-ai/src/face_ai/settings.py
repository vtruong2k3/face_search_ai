from functools import lru_cache
from pathlib import Path

from pydantic import field_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="FACE_AI_", extra="ignore")

    host: str = "0.0.0.0"
    port: int = 8001
    models_required: bool = False
    detector_model_path: Path | None = None
    detector_model_sha256: str | None = None
    embedder_model_path: Path | None = None
    embedder_model_sha256: str | None = None
    onnx_provider: str = "CPUExecutionProvider"
    insightface_enabled: bool = False
    insightface_model_root: Path | None = None
    insightface_pack: str = "buffalo_l"
    insightface_detection_width: int = 640
    insightface_detection_height: int = 640
    insightface_detection_threshold: float = 0.5
    qdrant_url: str = "http://localhost:6333"
    internal_token: str | None = None

    @field_validator("insightface_pack")
    @classmethod
    def validate_insightface_pack(cls, value: str) -> str:
        if value != "buffalo_l":
            raise ValueError("only the approved buffalo_l PoC pack is supported")
        return value

    @field_validator("insightface_detection_width", "insightface_detection_height")
    @classmethod
    def validate_detection_dimension(cls, value: int) -> int:
        if value <= 0:
            raise ValueError("InsightFace detection dimensions must be positive")
        return value

    @field_validator("insightface_detection_threshold")
    @classmethod
    def validate_detection_threshold(cls, value: float) -> float:
        if not 0.0 <= value <= 1.0:
            raise ValueError("InsightFace detection threshold must be between zero and one")
        return value

    @field_validator("detector_model_sha256", "embedder_model_sha256")
    @classmethod
    def validate_sha256(cls, value: str | None) -> str | None:
        if value is None:
            return None
        normalized = value.lower()
        if len(normalized) != 64 or any(character not in "0123456789abcdef" for character in normalized):
            raise ValueError("model checksum must be a hexadecimal SHA-256 value")
        return normalized


@lru_cache
def get_settings() -> Settings:
    return Settings()
