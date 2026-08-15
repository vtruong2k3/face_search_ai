from __future__ import annotations

import io
from dataclasses import dataclass
from enum import Enum

from PIL import Image, ImageOps, UnidentifiedImageError

from photo_worker.errors import (
    CorruptImageError,
    DecompressionRiskError,
    ImageDimensionError,
    ImageProcessingError,
    UnsupportedFormatError,
)

# Apply global decompression bomb limit
Image.MAX_IMAGE_PIXELS = 80_000_000

_FORMAT_MEDIA_TYPES = {
    "JPEG": "image/jpeg",
    "PNG": "image/png",
    "WEBP": "image/webp",
}


class DerivativeVariant(str, Enum):
    THUMBNAIL = "thumbnail"
    PREVIEW = "preview"


@dataclass(frozen=True, slots=True)
class DerivativeResult:
    variant: DerivativeVariant
    data: bytes
    media_type: str
    width: int
    height: int
    byte_size: int


class ImageProcessor:
    def __init__(
        self,
        max_image_bytes: int = 100 * 1024 * 1024,
        max_image_pixels: int = 80_000_000,
        max_image_dimension: int = 12_000,
        thumbnail_max_size: int = 400,
        thumbnail_quality: int = 80,
        preview_max_size: int = 1600,
        preview_quality: int = 85,
    ) -> None:
        self.max_image_bytes = max_image_bytes
        self.max_image_pixels = max_image_pixels
        self.max_image_dimension = max_image_dimension
        self.thumbnail_max_size = thumbnail_max_size
        self.thumbnail_quality = thumbnail_quality
        self.preview_max_size = preview_max_size
        self.preview_quality = preview_quality

    def _validate_and_load(self, content: bytes) -> Image.Image:
        if not content:
            raise CorruptImageError("image content is empty")
        if len(content) > self.max_image_bytes:
            raise ImageProcessingError(f"image exceeds the byte limit ({len(content)} > {self.max_image_bytes})")

        try:
            image = Image.open(io.BytesIO(content))
        except Image.DecompressionBombError as error:
            raise DecompressionRiskError("image exceeds pixel limit") from error
        except UnidentifiedImageError as error:
            raise UnsupportedFormatError("unsupported or corrupt image format") from error
        except OSError as error:
            raise CorruptImageError("truncated or corrupt image bytes") from error

        image_format = image.format or ""
        if image_format not in _FORMAT_MEDIA_TYPES:
            raise UnsupportedFormatError(f"unsupported image format: {image_format}")

        width, height = image.size
        if width <= 0 or height <= 0:
            raise ImageDimensionError("image dimensions must be positive")
        if (
            width > self.max_image_dimension
            or height > self.max_image_dimension
            or width * height > self.max_image_pixels
        ):
            raise DecompressionRiskError("image dimensions exceed configured maximums")

        try:
            # Apply EXIF orientation
            rotated = ImageOps.exif_transpose(image)
            if rotated is not None:
                image = rotated  # type: ignore[assignment]
        except Exception:
            # Ignore malformed EXIF data; preserve original image
            pass

        # Load the image completely into memory to catch truncation errors before processing
        try:
            image.load()
        except OSError as error:
            raise CorruptImageError("truncated or corrupt image bytes during load") from error

        # Normalize to RGB (or RGBA for transparent formats)
        if image.mode in ("RGBA", "LA", "P"):
            final_image = image.convert("RGBA")
        else:
            final_image = image.convert("RGB")

        # Create a fresh image to ensure all metadata/EXIF/IPTC is completely stripped
        clean_image = Image.new(final_image.mode, final_image.size)

        # Use get_flattened_data if available (Pillow >= 12), otherwise fallback for older versions
        if hasattr(final_image, "get_flattened_data"):
            data = list(final_image.get_flattened_data())
            clean_image.putdata(data)  # type: ignore[arg-type]
        else:
            data = list(final_image.getdata())
            clean_image.putdata(data)  # type: ignore[arg-type]

        return clean_image

    def _generate_variant(self, image: Image.Image, max_size: int, quality: int, variant: DerivativeVariant) -> DerivativeResult:
        width, height = image.size
        if width > max_size or height > max_size:
            # Downsample maintaining aspect ratio using high-quality Lanczos filter
            ratio = min(max_size / width, max_size / height)
            new_width = max(1, int(width * ratio))
            new_height = max(1, int(height * ratio))
            resized = image.resize((new_width, new_height), Image.Resampling.LANCZOS)
        else:
            resized = image

        out = io.BytesIO()
        # Save as WebP with metadata stripped (by default save doesn't copy EXIF unless passed)
        resized.save(out, format="WEBP", quality=quality, method=4)
        data = out.getvalue()

        return DerivativeResult(
            variant=variant,
            data=data,
            media_type="image/webp",
            width=resized.width,
            height=resized.height,
            byte_size=len(data),
        )

    def generate_derivatives(self, content: bytes) -> dict[DerivativeVariant, DerivativeResult]:
        """
        Validates content, strips metadata, and generates thumbnail and preview derivatives.
        """
        clean_image = self._validate_and_load(content)

        return {
            DerivativeVariant.PREVIEW: self._generate_variant(
                clean_image, self.preview_max_size, self.preview_quality, DerivativeVariant.PREVIEW
            ),
            DerivativeVariant.THUMBNAIL: self._generate_variant(
                clean_image, self.thumbnail_max_size, self.thumbnail_quality, DerivativeVariant.THUMBNAIL
            ),
        }
