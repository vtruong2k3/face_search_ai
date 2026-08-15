class PhotoWorkerError(Exception):
    """Base exception for photo worker."""


class ImageProcessingError(PhotoWorkerError):
    """Base exception for image processing failures."""


class DecompressionRiskError(ImageProcessingError):
    """Raised when an image exceeds decompression pixel/size limits."""


class CorruptImageError(ImageProcessingError):
    """Raised when image bytes are malformed or truncated."""


class UnsupportedFormatError(ImageProcessingError):
    """Raised when format is not JPEG, PNG, or WEBP."""


class ImageDimensionError(ImageProcessingError):
    """Raised when dimensions are non-positive or exceed bounds."""


class StorageError(PhotoWorkerError):
    """Base exception for object storage operations."""


class ObjectNotFoundError(StorageError):
    """Raised when requested object key does not exist in storage."""
