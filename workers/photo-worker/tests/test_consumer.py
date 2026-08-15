import asyncio
from typing import Any

import pytest

from photo_worker.consumer import WorkerConsumer
from photo_worker.jobs import JobEnvelope
from photo_worker.settings import Settings


class FakeRedis:
    def __init__(self) -> None:
        self.groups_created: list[tuple[str, str]] = []
        self.acked: list[tuple[str, str, str]] = []
        self.dlq_messages: list[tuple[str, dict[str, Any]]] = []

    async def xgroup_create(self, name: str, groupname: str, id: str = "$", mkstream: bool = False) -> bool:
        self.groups_created.append((name, groupname))
        return True

    async def xack(self, name: str, groupname: str, *ids: str) -> int:
        for mid in ids:
            self.acked.append((name, groupname, mid))
        return len(ids)

    async def xadd(self, name: str, fields: dict[str, Any]) -> str:
        self.dlq_messages.append((name, fields))
        return "1-0"


@pytest.mark.asyncio
async def test_consumer_successful_dispatch() -> None:
    fake_redis = FakeRedis()
    settings = Settings(max_concurrency=2, max_retries=2)
    processed: list[str] = []

    async def mock_processor(job: JobEnvelope) -> None:
        processed.append(job.type)

    consumer = WorkerConsumer(client=fake_redis, settings=settings, processor=mock_processor)  # type: ignore[arg-type]
    fields = {b"type": b"connectivity.smoke", b"payload": b"{}"}

    await consumer._dispatch("100-0", fields)
    # wait for tasks to finish
    await asyncio.gather(*consumer._active_tasks, return_exceptions=True)

    assert processed == ["connectivity.smoke"]
    assert ("photo-jobs", "photo-workers", "100-0") in fake_redis.acked


@pytest.mark.asyncio
async def test_consumer_retry_and_dead_letter_on_failure() -> None:
    fake_redis = FakeRedis()
    settings = Settings(max_concurrency=2, max_retries=2)
    attempts = 0

    async def failing_processor(job: JobEnvelope) -> None:
        nonlocal attempts
        attempts += 1
        raise RuntimeError("simulated processing error")

    consumer = WorkerConsumer(client=fake_redis, settings=settings, processor=failing_processor)  # type: ignore[arg-type]
    fields = {b"type": b"connectivity.smoke", b"payload": b"{}"}

    await consumer._dispatch("200-0", fields)
    await asyncio.gather(*consumer._active_tasks, return_exceptions=True)

    assert attempts == 2
    assert len(fake_redis.dlq_messages) == 1
    dlq_stream, dlq_data = fake_redis.dlq_messages[0]
    assert dlq_stream == "photo-jobs-dlq"
    assert dlq_data["message_id"] == "200-0"
    assert "simulated processing error" in dlq_data["error"]
    assert ("photo-jobs", "photo-workers", "200-0") in fake_redis.acked


@pytest.mark.asyncio
async def test_consumer_malformed_job_goes_to_dlq() -> None:
    fake_redis = FakeRedis()
    settings = Settings(max_concurrency=2, max_retries=2)

    consumer = WorkerConsumer(client=fake_redis, settings=settings)  # type: ignore[arg-type]
    bad_fields = {b"invalid_field": b"corrupt"}

    await consumer._dispatch("300-0", bad_fields)
    await asyncio.gather(*consumer._active_tasks, return_exceptions=True)

    assert len(fake_redis.dlq_messages) == 1
    dlq_stream, dlq_data = fake_redis.dlq_messages[0]
    assert dlq_stream == "photo-jobs-dlq"
    assert dlq_data["message_id"] == "300-0"
    assert ("photo-jobs", "photo-workers", "300-0") in fake_redis.acked
