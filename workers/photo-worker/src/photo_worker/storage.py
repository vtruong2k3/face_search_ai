import asyncio
import io
import re
from dataclasses import dataclass
from typing import Any

from minio import Minio
from minio.deleteobjects import DeleteObject
from minio.error import S3Error

from photo_worker.errors import ObjectNotFoundError, StorageError


def safe_segment(value: str) -> bool:
    if not value or value != value.strip():
        return False
    if "/" in value or "\\" in value:
        return False
    return True


def build_photo_prefix(organization_id: str, event_id: str, photo_id: str) -> str:
    """Object-key prefix that contains a single photo's original and all of its
    derivatives. The trailing slash ensures a prefix match cannot spill into a
    sibling photo whose id shares a leading substring."""
    if not all(safe_segment(v) for v in (organization_id, event_id, photo_id)):
        raise ValueError("invalid path segment in photo prefix construction")
    return f"organizations/{organization_id}/events/{event_id}/photos/{photo_id}/"


def build_event_prefix(organization_id: str, event_id: str) -> str:
    """Object-key prefix that contains every photo (originals and derivatives)
    for an event."""
    if not all(safe_segment(v) for v in (organization_id, event_id)):
        raise ValueError("invalid path segment in event prefix construction")
    return f"organizations/{organization_id}/events/{event_id}/"


def build_object_key(organization_id: str, event_id: str, photo_id: str, variant: str) -> str:
    if not all(safe_segment(v) for v in (organization_id, event_id, photo_id, variant)):
        raise ValueError("invalid path segment in object key construction")

    # Matching apps/api/internal/domain/photo/photo.go deterministic key logic
    if variant == "original":
        return f"organizations/{organization_id}/events/{event_id}/photos/{photo_id}/original"

    # Derivatives go into a variants namespace to avoid collisions and allow policy scoping
    if not re.match(r"^[a-z0-9\-\.]+$", variant):
        raise ValueError("invalid variant name")

    return f"organizations/{organization_id}/events/{event_id}/photos/{photo_id}/{variant}"


@dataclass(frozen=True, slots=True)
class ObjectMetadata:
    bucket: str
    object_key: str
    size: int
    content_type: str


class MinioStorageAdapter:
    def __init__(self, client: Minio) -> None:
        self.client = client

    async def ensure_bucket(self, bucket: str) -> None:
        def _check_and_make() -> None:
            if not self.client.bucket_exists(bucket):
                self.client.make_bucket(bucket)

        try:
            await asyncio.to_thread(_check_and_make)
        except S3Error as error:
            raise StorageError(f"failed to ensure bucket {bucket}: {error}") from error

    async def get_object(self, bucket: str, object_key: str) -> bytes:
        def _get() -> bytes:
            response = None
            try:
                response = self.client.get_object(bucket, object_key)
                return response.read()
            finally:
                if response is not None:
                    response.close()
                    response.release_conn()

        try:
            return await asyncio.to_thread(_get)
        except S3Error as error:
            if error.code in ("NoSuchKey", "NoSuchBucket"):
                raise ObjectNotFoundError(f"object {object_key} not found in {bucket}") from error
            raise StorageError(f"failed to get object {object_key}: {error}") from error
        except Exception as error:
            raise StorageError(f"unexpected error fetching {object_key}: {error}") from error

    async def put_object(self, bucket: str, object_key: str, data: bytes, content_type: str) -> None:
        def _put() -> None:
            data_stream = io.BytesIO(data)
            self.client.put_object(
                bucket_name=bucket,
                object_name=object_key,
                data=data_stream,
                length=len(data),
                content_type=content_type,
            )

        try:
            await asyncio.to_thread(_put)
        except S3Error as error:
            raise StorageError(f"failed to put object {object_key}: {error}") from error
        except Exception as error:
            raise StorageError(f"unexpected error putting {object_key}: {error}") from error

    async def remove_prefix(self, bucket: str, prefix: str) -> int:
        """Remove every object under ``prefix``. Refuses an empty or bucket-wide
        prefix so a deletion job can never wipe an entire bucket. Idempotent:
        removing an already-empty prefix returns 0 and raises nothing. Returns the
        number of objects removed."""
        if not prefix or not prefix.endswith("/") or "organizations/" not in prefix:
            raise StorageError("refusing to remove objects for an unscoped prefix")

        def _remove() -> int:
            objects = self.client.list_objects(bucket, prefix=prefix, recursive=True)
            names = [obj.object_name for obj in objects if obj.object_name]
            if not names:
                return 0
            errors = list(self.client.remove_objects(bucket, [DeleteObject(name) for name in names]))
            if errors:
                # Do not surface object paths; report only the count.
                raise StorageError(f"failed to remove {len(errors)} objects under prefix")
            return len(names)

        try:
            return await asyncio.to_thread(_remove)
        except StorageError:
            raise
        except S3Error as error:
            raise StorageError(f"failed to remove objects under prefix: {error}") from error
        except Exception as error:
            raise StorageError(f"unexpected error removing objects under prefix: {error}") from error

    async def stat_object(self, bucket: str, object_key: str) -> ObjectMetadata:
        def _stat() -> Any:
            return self.client.stat_object(bucket, object_key)

        try:
            result = await asyncio.to_thread(_stat)
            return ObjectMetadata(
                bucket=result.bucket_name,
                object_key=result.object_name,
                size=result.size,
                content_type=result.content_type,
            )
        except S3Error as error:
            if error.code in ("NoSuchKey", "NoSuchBucket"):
                raise ObjectNotFoundError(f"object {object_key} not found in {bucket}") from error
            raise StorageError(f"failed to stat object {object_key}: {error}") from error
        except Exception as error:
            raise StorageError(f"unexpected error statting {object_key}: {error}") from error
