import asyncio
import signal

import httpx
import redis.asyncio as redis
import structlog
from redis.exceptions import ResponseError

from photo_worker.jobs import JobEnvelope
from photo_worker.settings import Settings

log = structlog.get_logger()


async def process(job: JobEnvelope, face_ai_url: str) -> None:
    if job.type != "connectivity.smoke":
        raise ValueError(f"unsupported job type: {job.type}")
    async with httpx.AsyncClient(timeout=5) as client:
        response = await client.get(f"{face_ai_url.rstrip('/')}/health/ready")
        response.raise_for_status()


async def run() -> None:
    settings = Settings()
    client = redis.from_url(settings.redis_url, decode_responses=False)
    try:
        try:
            await client.xgroup_create(settings.redis_stream, settings.redis_group, id="0", mkstream=True)
        except ResponseError as error:
            if "BUSYGROUP" not in str(error):
                raise
        log.info("photo_worker_started", stream=settings.redis_stream, group=settings.redis_group)
        while True:
            batches = await client.xreadgroup(settings.redis_group, settings.worker_name, {settings.redis_stream: ">"}, count=1, block=settings.block_ms)
            for _, messages in batches:
                for message_id, fields in messages:
                    try:
                        job = JobEnvelope.from_stream(fields)
                        await process(job, settings.face_ai_url)
                        await client.xack(settings.redis_stream, settings.redis_group, message_id)
                        log.info("job_completed", job_id=job.job_id, type=job.type)
                    except Exception as error:  # noqa: BLE001 - failed jobs must reach the DLQ
                        await client.xadd(settings.dead_letter_stream, {"message_id": message_id, "error": str(error), "job": fields.get(b"job", b"")})
                        await client.xack(settings.redis_stream, settings.redis_group, message_id)
                        log.error("job_dead_lettered", message_id=message_id, error=str(error))
    finally:
        await client.aclose()


def main() -> None:
    structlog.configure(processors=[structlog.processors.TimeStamper(fmt="iso"), structlog.processors.JSONRenderer()])
    loop = asyncio.new_event_loop()
    task = loop.create_task(run())
    for signum in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(signum, task.cancel)
    try:
        loop.run_until_complete(task)
    except asyncio.CancelledError:
        log.info("photo_worker_stopped")
    finally:
        loop.close()
