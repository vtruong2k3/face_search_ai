import asyncio
import signal

import redis.asyncio as redis
import structlog

from photo_worker.consumer import WorkerConsumer
from photo_worker.settings import Settings

log = structlog.get_logger()


async def run() -> None:
    settings = Settings()
    client = redis.from_url(settings.redis_url, decode_responses=False)
    consumer = WorkerConsumer(client=client, settings=settings)
    try:
        await consumer.start()
    finally:
        await client.aclose()


def main() -> None:
    structlog.configure(
        processors=[
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.JSONRenderer(),
        ]
    )
    loop = asyncio.new_event_loop()
    task = loop.create_task(run())
    for signum in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(signum, task.cancel)
        except NotImplementedError:
            pass
    try:
        loop.run_until_complete(task)
    except asyncio.CancelledError:
        log.info("photo_worker_stopped")
    finally:
        loop.close()
