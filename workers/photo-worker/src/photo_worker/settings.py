from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(extra="ignore", env_file=".env")

    redis_url: str = "redis://localhost:6379/0"
    redis_stream: str = "photo-jobs"
    redis_group: str = "photo-workers"
    worker_name: str = "photo-worker-1"
    dead_letter_stream: str = "photo-jobs-dlq"
    face_ai_url: str = "http://localhost:8001"
    block_ms: int = 2000
    batch_size: int = 10
    max_concurrency: int = 5
    max_retries: int = 3
    claim_min_idle_ms: int = 30000
    claim_interval_s: float = 15.0
