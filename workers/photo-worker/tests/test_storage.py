import pytest

from photo_worker.storage import build_object_key


def test_build_object_key_original() -> None:
    key = build_object_key("org-1", "evt-1", "photo-1", "original")
    assert key == "organizations/org-1/events/evt-1/photos/photo-1/original"


def test_build_object_key_derivative() -> None:
    key = build_object_key("org-1", "evt-1", "photo-1", "thumbnail")
    assert key == "organizations/org-1/events/evt-1/photos/photo-1/thumbnail"

    key = build_object_key("org-1", "evt-1", "photo-1", "preview")
    assert key == "organizations/org-1/events/evt-1/photos/photo-1/preview"


def test_build_object_key_invalid_segments() -> None:
    # Invalid characters
    with pytest.raises(ValueError, match="invalid path segment"):
        build_object_key("org/1", "evt-1", "photo-1", "original")

    # Empty segments
    with pytest.raises(ValueError, match="invalid path segment"):
        build_object_key("org-1", "  ", "photo-1", "original")

    # Invalid variant
    with pytest.raises(ValueError, match="invalid path segment|invalid variant name"):
        build_object_key("org-1", "evt-1", "photo-1", "invalid/variant")
