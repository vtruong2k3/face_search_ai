from __future__ import annotations

from dataclasses import dataclass
from io import BytesIO

import numpy as np
from PIL import Image, UnidentifiedImageError

from face_ai.domain import ImageArray


class ImageValidationError(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class ImageValidationLimits:
    max_bytes: int = 20 * 1024 * 1024
    max_width: int = 12_000
    max_height: int = 12_000
    max_pixels: int = 80_000_000

    def __post_init__(self) -> None:
        if min(self.max_bytes, self.max_width, self.max_height, self.max_pixels) <= 0:
            raise ValueError("image validation limits must be positive")


@dataclass(frozen=True, slots=True)
class ValidatedImage:
    pixels: ImageArray
    media_type: str
    width: int
    height: int


_FORMAT_MEDIA_TYPES = {
    "JPEG": "image/jpeg",
    "PNG": "image/png",
    "WEBP": "image/webp",
}


def validate_image(
    content: bytes,
    *,
    limits: ImageValidationLimits | None = None,
) -> ValidatedImage:
    limits = limits or ImageValidationLimits()
    if not content:
        raise ImageValidationError("image content is empty")
    if len(content) > limits.max_bytes:
        raise ImageValidationError("image exceeds the byte limit")

    try:
        with Image.open(BytesIO(content)) as image:
            image_format = image.format
            media_type = _FORMAT_MEDIA_TYPES.get(image_format or "")
            if media_type is None:
                raise ImageValidationError("image format is unsupported")

            width, height = image.size
            if width <= 0 or height <= 0:
                raise ImageValidationError("image dimensions must be positive")
            if width > limits.max_width or height > limits.max_height or width * height > limits.max_pixels:
                raise ImageValidationError("image dimensions exceed configured limits")

            image.load()
            pixels = np.asarray(image.convert("RGB"), dtype=np.uint8)
    except ImageValidationError:
        raise
    except (Image.DecompressionBombError, UnidentifiedImageError, OSError, ValueError) as error:
        raise ImageValidationError("image content is unsupported or corrupt") from error

    pixels.setflags(write=False)
    return ValidatedImage(pixels=pixels, media_type=media_type, width=width, height=height)
