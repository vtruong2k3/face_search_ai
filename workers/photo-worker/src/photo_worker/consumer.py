import asyncio
from collections.abc import Callable, Coroutine
from typing import Any

import redis.asyncio as redis
import structlog
from minio import Minio
from psycopg_pool import ConnectionPool
from qdrant_client import QdrantClient
from redis.exceptions import ResponseError

from photo_worker.errors import TerminalProcessingError
from photo_worker.face_ai import FaceAIClient
from photo_worker.image import ImageProcessor
from photo_worker.index import FaceVectorIndex
from photo_worker.jobs import JobEnvelope, PhotoProcessingPayload
from photo_worker.persist import PostgresPhotoPersistence
from photo_worker.processor import PhotoProcessor
from photo_worker.settings import Settings
from photo_worker.storage import MinioStorageAdapter

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

        minio_client = Minio(
            endpoint=settings.minio_endpoint,
            access_key=settings.minio_access_key,
            secret_key=settings.minio_secret_key,
            secure=settings.minio_secure,
        )
        self.storage = MinioStorageAdapter(minio_client)
        self.image_processor = ImageProcessor(
            max_image_bytes=settings.max_image_bytes,
            max_image_pixels=settings.max_image_pixels,
            max_image_dimension=settings.max_image_dimension,
            thumbnail_max_size=settings.thumbnail_max_size,
            thumbnail_quality=settings.thumbnail_quality,
            preview_max_size=settings.preview_max_size,
            preview_quality=settings.preview_quality,
        )
        self.photo_processor: PhotoProcessor | None = None
        self._db_pool: ConnectionPool | None = None
        self._qdrant: QdrantClient | None = None
        self._face_ai: FaceAIClient | None = None
        if processor is None:
            self._db_pool = ConnectionPool(
                conninfo=settings.database_url,
                min_size=settings.database_min_pool,
                max_size=settings.database_max_pool,
                open=False,
            )
            self._db_pool.open(wait=False)
            self._qdrant = QdrantClient(url=settings.qdrant_url)
            self._face_ai = FaceAIClient(
                base_url=settings.face_ai_url,
                internal_token=settings.face_ai_internal_token,
                timeout_s=settings.face_ai_timeout_s,
                embedding_dimension=settings.embedding_dimension,
            )
            self.photo_processor = PhotoProcessor(
                settings=settings,
                storage=self.storage,
                image_processor=self.image_processor,
                face_ai=self._face_ai,
                index=FaceVectorIndex(
                    client=self._qdrant,  # type: ignore[arg-type]
                    collection_name=settings.qdrant_collection,
                    dimension=settings.embedding_dimension,
                ),
                persist=PostgresPhotoPersistence(self._db_pool),
            )

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
            if self._face_ai is not None:
                await self._face_ai.aclose()
            if self._db_pool is not None:
                self._db_pool.close()
            if self._qdrant is not None:
                self._qdrant.close()
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

                for batch in batches:
                    if not isinstance(batch, (tuple, list)) or len(batch) < 2:
                        continue
                    messages = batch[1]
                    if not isinstance(messages, (tuple, list)):
                        continue
                    for msg in messages:
                        if not isinstance(msg, (tuple, list)) or len(msg) < 2:
                            continue
                        message_id: Any = msg[0]
                        fields: Any = msg[1]
                        if not isinstance(fields, dict):
                            continue
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
                if not isinstance(res, (tuple, list)) or len(res) < 2:
                    continue

                next_start_id: Any = res[0]
                messages_batch: Any = res[1]
                start_id = next_start_id.decode("utf-8") if isinstance(next_start_id, bytes) else str(next_start_id)

                if not isinstance(messages_batch, (tuple, list)):
                    continue

                for msg in messages_batch:
                    if not isinstance(msg, (tuple, list)) or len(msg) < 2:
                        continue
                    message_id_raw: Any = msg[0]
                    fields_raw: Any = msg[1]
                    if not isinstance(fields_raw, dict):
                        continue
                    msg_id_str = message_id_raw.decode("utf-8") if isinstance(message_id_raw, bytes) else str(message_id_raw)
                    log.info("message_autoclaimed", message_id=msg_id_str)
                    await self._dispatch(msg_id_str, fields_raw)
            except asyncio.CancelledError:
                break
            except Exception as error:
                log.error("autoclaim_error", error=str(error))

    async def _dispatch(self, message_id: str, fields: dict[Any, Any]) -> None:
        await self._semaphore.acquire()
        task = asyncio.create_task(self._process_message_wrapper(message_id, fields))
        self._active_tasks.add(task)

        def _task_done(t: asyncio.Task[None]) -> None:
            self._active_tasks.discard(t)
            self._semaphore.release()

        task.add_done_callback(_task_done)

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
            except TerminalProcessingError as err:
                log.warning(
                    "terminal_photo_processing_failure",
                    message_id=message_id,
                    correlation_id=correlation_id,
                    error=str(err),
                )
                try:
                    await self._fail_terminal_photo(job, err)
                    await self.client.xack(self.settings.redis_stream, self.settings.redis_group, message_id)
                except Exception:
                    await self._dead_letter(message_id, fields, str(err))
                return
            except Exception as err:
                log.warning(
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
            dlq_fields: dict[str, str] = {
                "original_stream": self.settings.redis_stream,
                "message_id": message_id,
                "error": error[:256],
            }
            if isinstance(fields, dict):
                for k, v in fields.items():
                    k_str = k.decode("utf-8") if isinstance(k, bytes) else str(k)
                    v_val = v.decode("utf-8") if isinstance(v, bytes) else str(v)
                    dlq_fields[f"field_{k_str}"] = v_val

            # Redis xadd expects keys and values to be bytes, string, int, float
            await self.client.xadd(self.settings.dead_letter_stream, dlq_fields)  # type: ignore[arg-type]
            await self.client.xack(self.settings.redis_stream, self.settings.redis_group, message_id)
            log.error("job_sent_to_dlq", message_id=message_id, error=error[:256])
        except Exception as dlq_err:
            log.error("failed_to_dead_letter", message_id=message_id, error=str(dlq_err))

    async def default_process(self, job: JobEnvelope) -> None:
        if job.type == "connectivity.smoke":
            return
        if job.type == "photo.processing.requested" and isinstance(job.payload, PhotoProcessingPayload):
            if self.photo_processor is None:
                raise RuntimeError("photo processor is not configured")
            await self.photo_processor.process(job.payload)
            return

        raise ValueError(f"unsupported job type: {job.type}")

    async def _fail_terminal_photo(self, job: JobEnvelope, error: Exception) -> None:
        if not isinstance(job.payload, PhotoProcessingPayload) or self.photo_processor is None:
            return
        try:
            await asyncio.to_thread(
                self.photo_processor.fail_photo,
                job.payload,
                type(error).__name__.lower(),
            )
        except Exception as failure_error:
            log.error(
                "failed_to_mark_photo_failed",
                photo_id=job.payload.photo_id,
                error=str(failure_error),
            )
            raise
