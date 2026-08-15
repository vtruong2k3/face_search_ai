import asyncio
import structlog
from typing import Any, Callable, Coroutine
import redis.asyncio as redis
from redis.exceptions import ResponseError

from photo_worker.jobs import JobEnvelope, PhotoProcessingPayload
from photo_worker.settings import Settings

log = structlog.get_logger()


class WorkerConsumer:
    def __init__(
        self,
        client: redis.Redis,
        settings: Settings,
        processor: Callable[[JobEnvelope], Coroutine[Any, Any, None]] | None = None,
    ) -> None:
        self.client = client
        self.settings = settings
        self.processor = processor
        self._running = False
        self._semaphore = asyncio.Semaphore(settings.max_concurrency)
        self._active_tasks: set[asyncio.Task[None]] = set()

    async def setup_group(self) -> None:
        try:
            await self.client.xgroup_create(
                self.settings.redis_stream,
                self.settings.redis_group,
                id="0",
                mkstream=True,
            )
            log.info("consumer_group_created", stream=self.settings.redis_stream, group=self.settings.redis_group)
        except ResponseError as error:
            if "BUSYGROUP" not in str(error):
                raise

    async def start(self) -> None:
        self._running = True
        await self.setup_group()
        log.info(
            "worker_consumer_started",
            worker=self.settings.worker_name,
            stream=self.settings.redis_stream,
            group=self.settings.redis_group,
            max_concurrency=self.settings.max_concurrency,
        )

        autoclaim_task = asyncio.create_task(self._autoclaim_loop())
        read_task = asyncio.create_task(self._read_loop())

        try:
            await asyncio.gather(autoclaim_task, read_task)
        except asyncio.CancelledError:
            log.info("worker_consumer_stopping")
        finally:
            self._running = False
            autoclaim_task.cancel()
            read_task.cancel()
            await asyncio.gather(autoclaim_task, read_task, return_exceptions=True)
            if self._active_tasks:
                await asyncio.gather(*self._active_tasks, return_exceptions=True)
            log.info("worker_consumer_stopped")

    def stop(self) -> None:
        self._running = False

    async def _read_loop(self) -> None:
        while self._running:
            try:
                batches = await self.client.xreadgroup(
                    groupname=self.settings.redis_group,
                    consumername=self.settings.worker_name,
                    streams={self.settings.redis_stream: ">"},
                    count=self.settings.batch_size,
                    block=self.settings.block_ms,
                )
                if not batches:
                    continue

                for _, messages in batches:
                    for message_id, fields in messages:
                        msg_id_str = message_id.decode("utf-8") if isinstance(message_id, bytes) else str(message_id)
                        await self._dispatch(msg_id_str, fields)
            except asyncio.CancelledError:
                break
            except Exception as error:
                log.error("xreadgroup_error", error=str(error))
                await asyncio.sleep(1)

    async def _autoclaim_loop(self) -> None:
        start_id = "0-0"
        while self._running:
            try:
                await asyncio.sleep(self.settings.claim_interval_s)
                res = await self.client.xautoclaim(
                    name=self.settings.redis_stream,
                    groupname=self.settings.redis_group,
                    consumername=self.settings.worker_name,
                    min_idle_time=self.settings.claim_min_idle_ms,
                    start_id=start_id,
                    count=self.settings.batch_size,
                )
                next_start_id, messages = res[0], res[1]
                start_id = next_start_id.decode("utf-8") if isinstance(next_start_id, bytes) else str(next_start_id)

                for message_id, fields in messages:
                    msg_id_str = message_id.decode("utf-8") if isinstance(message_id, bytes) else str(message_id)
                    log.info("message_autoclaimed", message_id=msg_id_str)
                    await self._dispatch(msg_id_str, fields)
            except asyncio.CancelledError:
                break
            except Exception as error:
                log.error("autoclaim_error", error=str(error))

    async def _dispatch(self, message_id: str, fields: dict[Any, Any]) -> None:
        await self._semaphore.acquire()
        task = asyncio.create_task(self._process_message_wrapper(message_id, fields))
        self._active_tasks.add(task)
        task.add_done_callback(lambda t: (self._active_tasks.discard(t), self._semaphore.release()))

    async def _process_message_wrapper(self, message_id: str, fields: dict[Any, Any]) -> None:
        try:
            job = JobEnvelope.from_stream(fields)
        except Exception as parse_error:
            log.error("job_parse_failed", message_id=message_id, error=str(parse_error))
            await self._dead_letter(message_id, fields, str(parse_error))
            return

        correlation_id = ""
        if isinstance(job.payload, PhotoProcessingPayload):
            correlation_id = job.payload.photo_id

        attempt = 0
        while attempt < self.settings.max_retries:
            attempt += 1
            try:
                if self.processor:
                    await self.processor(job)
                else:
                    await self.default_process(job)

                await self.client.xack(self.settings.redis_stream, self.settings.redis_group, message_id)
                log.info("job_processed_successfully", message_id=message_id, correlation_id=correlation_id, type=job.type)
                return
            except Exception as err:
                log.warn(
                    "job_processing_attempt_failed",
                    message_id=message_id,
                    correlation_id=correlation_id,
                    attempt=attempt,
                    max_retries=self.settings.max_retries,
                    error=str(err),
                )
                if attempt < self.settings.max_retries:
                    backoff_s = min(2 ** (attempt - 1), 30)
                    await asyncio.sleep(backoff_s)
                else:
                    await self._dead_letter(message_id, fields, str(err))

    async def _dead_letter(self, message_id: str, fields: dict[Any, Any], error: str) -> None:
        try:
            dlq_fields = {
                "original_stream": self.settings.redis_stream,
                "message_id": message_id,
                "error": error[:256],
            }
            if isinstance(fields, dict):
                for k, v in fields.items():
                    k_str = k.decode("utf-8") if isinstance(k, bytes) else str(k)
                    v_val = v.decode("utf-8") if isinstance(v, bytes) else str(v)
                    dlq_fields[f"field_{k_str}"] = v_val

            await self.client.xadd(self.settings.dead_letter_stream, dlq_fields)
            await self.client.xack(self.settings.redis_stream, self.settings.redis_group, message_id)
            log.error("job_sent_to_dlq", message_id=message_id, error=error[:256])
        except Exception as dlq_err:
            log.error("failed_to_dead_letter", message_id=message_id, error=str(dlq_err))

    async def default_process(self, job: JobEnvelope) -> None:
        if job.type == "connectivity.smoke":
            return
        if job.type == "photo.processing.requested":
            # Will be expanded with Face AI inference, derivatives and Qdrant in subsequent Tasks
            log.info("received_photo_processing_job", type=job.type)
            return
        raise ValueError(f"unsupported job type: {job.type}")
