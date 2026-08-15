import io

import pytest
from PIL import Image

from photo_worker.errors import (
    CorruptImageError,
    DecompressionRiskError,
    ImageProcessingError,
    UnsupportedFormatError,
)
from photo_worker.image import DerivativeVariant, ImageProcessor


def create_test_image_bytes(width: int, height: int, format: str = "JPEG", mode: str = "RGB") -> bytes:
    img = Image.new(mode, (width, height), color="red")
    out = io.BytesIO()
    img.save(out, format=format)
    return out.getvalue()


def test_valid_image_derivatives_generated() -> None:
    processor = ImageProcessor(
        thumbnail_max_size=400,
        preview_max_size=1600,
    )
    # Create an image that is larger than preview max size to test downsampling
    image_bytes = create_test_image_bytes(3200, 1600, format="JPEG")

    derivatives = processor.generate_derivatives(image_bytes)

    assert DerivativeVariant.THUMBNAIL in derivatives
    assert DerivativeVariant.PREVIEW in derivatives

    thumb = derivatives[DerivativeVariant.THUMBNAIL]
    assert thumb.variant == DerivativeVariant.THUMBNAIL
    assert thumb.media_type == "image/webp"
    # Aspect ratio should be preserved: 3200x1600 -> 400x200
    assert thumb.width == 400
    assert thumb.height == 200

    prev = derivatives[DerivativeVariant.PREVIEW]
    assert prev.variant == DerivativeVariant.PREVIEW
    assert prev.media_type == "image/webp"
    # Aspect ratio should be preserved: 3200x1600 -> 1600x800
    assert prev.width == 1600
    assert prev.height == 800


def test_small_image_not_upscaled() -> None:
    processor = ImageProcessor(thumbnail_max_size=400, preview_max_size=1600)
    # Smaller than thumbnail max size
    image_bytes = create_test_image_bytes(200, 100, format="PNG")

    derivatives = processor.generate_derivatives(image_bytes)
    thumb = derivatives[DerivativeVariant.THUMBNAIL]
    prev = derivatives[DerivativeVariant.PREVIEW]

    # Should not upscale
    assert thumb.width == 200
    assert thumb.height == 100
    assert prev.width == 200
    assert prev.height == 100


def test_empty_content_raises_error() -> None:
    processor = ImageProcessor()
    with pytest.raises(CorruptImageError, match="empty"):
        processor.generate_derivatives(b"")


def test_exceeds_byte_limit_raises_error() -> None:
    processor = ImageProcessor(max_image_bytes=100)
    image_bytes = create_test_image_bytes(10, 10, format="JPEG")
    # Make sure we created something > 100 bytes
    assert len(image_bytes) > 100

    with pytest.raises(ImageProcessingError, match="byte limit"):
        processor.generate_derivatives(image_bytes)


def test_unsupported_format_raises_error() -> None:
    processor = ImageProcessor()
    # GIF is not in our whitelist
    image_bytes = create_test_image_bytes(100, 100, format="GIF")

    with pytest.raises(UnsupportedFormatError, match="unsupported image format"):
        processor.generate_derivatives(image_bytes)


def test_exceeds_dimension_limit_raises_error() -> None:
    processor = ImageProcessor(max_image_dimension=1000)
    # Width exceeds 1000
    image_bytes = create_test_image_bytes(1001, 100, format="JPEG")

    with pytest.raises(DecompressionRiskError, match="dimensions exceed"):
        processor.generate_derivatives(image_bytes)


def test_corrupt_image_bytes_raises_error() -> None:
    processor = ImageProcessor()
    image_bytes = create_test_image_bytes(100, 100, format="JPEG")
    # Truncate the file heavily
    corrupt_bytes = image_bytes[: len(image_bytes) // 2]

    with pytest.raises(CorruptImageError, match="truncated or corrupt"):
        processor.generate_derivatives(corrupt_bytes)


def test_transparent_image_handled_correctly() -> None:
    processor = ImageProcessor()
    image_bytes = create_test_image_bytes(100, 100, format="PNG", mode="RGBA")

    derivatives = processor.generate_derivatives(image_bytes)
    thumb = derivatives[DerivativeVariant.THUMBNAIL]
    assert thumb.media_type == "image/webp"

    # Verify we can open the resulting webp
    result_img = Image.open(io.BytesIO(thumb.data))
    assert result_img.mode in ("RGB", "RGBA")


def test_exif_metadata_stripped() -> None:
    # Create a JPEG with some EXIF data
    img = Image.new("RGB", (100, 100), color="blue")

    # We will use piexif or just PIL's Exif object to inject mock EXIF
    exif = img.getexif()
    # 271 is Make, 272 is Model
    exif[271] = "SecretCamera"
    exif[272] = "ModelX"

    out = io.BytesIO()
    img.save(out, format="JPEG", exif=exif)
    image_bytes = out.getvalue()

    # Ensure our test image actually has EXIF
    test_img = Image.open(io.BytesIO(image_bytes))
    assert test_img.getexif()

    processor = ImageProcessor()
    derivatives = processor.generate_derivatives(image_bytes)

    # Resulting WebP images should have NO EXIF metadata
    for variant in derivatives.values():
        result_img = Image.open(io.BytesIO(variant.data))
        # WebP doesn't use the standard Exif dictionary in PIL the same way,
        # but calling getexif() should return an empty mapping.
        assert not result_img.getexif()
