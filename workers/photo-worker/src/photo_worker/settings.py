from __future__ import annotations

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(extra="ignore", env_file=".env")

    redis_url: str = "redis://localhost:6379/0"
    redis_stream: str = "photo-jobs"
    redis_group: str = "photo-workers"
    worker_name: str = "photo-worker-1"
    dead_letter_stream: str = "photo-jobs-dlq"
    face_ai_url: str = "http://localhost:8001"
    face_ai_internal_token: str = ""
    face_ai_timeout_s: float = 30.0
    block_ms: int = 2000
    batch_size: int = 10
    max_concurrency: int = 5
    max_retries: int = 3
    claim_min_idle_ms: int = 30000
    claim_interval_s: float = 15.0

    database_url: str = "postgres://postgres:postgres@localhost:5432/face_search?sslmode=disable"
    database_min_pool: int = 1
    database_max_pool: int = 5

    qdrant_url: str = "http://localhost:6333"
    qdrant_collection: str = "face-search-faces"
    embedding_dimension: int = 512

    minio_endpoint: str = "localhost:9000"
    minio_access_key: str = "minioadmin"
    minio_secret_key: str = "minioadmin"
    minio_secure: bool = False
    minio_bucket: str = "face-search"

    thumbnail_max_size: int = 400
    thumbnail_quality: int = 80
    preview_max_size: int = 1600
    preview_quality: int = 85
    max_image_bytes: int = 100 * 1024 * 1024
    max_image_pixels: int = 80_000_000
    max_image_dimension: int = 12_000
