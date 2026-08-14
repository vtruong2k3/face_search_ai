from __future__ import annotations

from dataclasses import asdict, fields

import pytest

from face_ai.benchmark.runtime_metadata import RuntimeMetadata, collect_runtime_metadata
from face_ai.settings import Settings


def metadata() -> RuntimeMetadata:
    return RuntimeMetadata(
        operating_system="Linux",
        architecture="x86_64",
        logical_cpu_count=8,
        python_version="3.11.9",
        onnx_provider="CPUExecutionProvider",
        onnxruntime_version="1.28.0",
        insightface_version="0.7.3",
        numpy_version="2.4.6",
        opencv_version="5.0.0",
        pillow_version="12.3.0",
        qdrant_client_version="1.19.0",
        insightface_pack="buffalo_l",
        detection_width=640,
        detection_height=640,
        detection_threshold=0.5,
        execution_policy="serial",
    )


def test_runtime_metadata_has_exact_sanitized_fields() -> None:
    result = asdict(metadata())

    assert tuple(result) == tuple(field.name for field in fields(RuntimeMetadata))
    assert set(result) == {
        "operating_system", "architecture", "logical_cpu_count", "python_version",
        "onnx_provider", "onnxruntime_version", "insightface_version", "numpy_version",
        "opencv_version", "pillow_version", "qdrant_client_version", "insightface_pack",
        "detection_width", "detection_height", "detection_threshold", "execution_policy",
    }
    assert not ({"hostname", "username", "model_root", "qdrant_url", "environment"} & set(result))


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("operating_system", ""),
        ("architecture", " "),
        ("logical_cpu_count", 0),
        ("detection_width", 0),
        ("detection_height", -1),
        ("detection_threshold", float("nan")),
        ("detection_threshold", 1.1),
        ("execution_policy", "parallel"),
    ],
)
def test_runtime_metadata_rejects_invalid_values(field: str, value: object) -> None:
    values = asdict(metadata())
    values[field] = value

    with pytest.raises(ValueError):
        RuntimeMetadata(**values)


def test_collect_runtime_metadata_uses_only_sanitized_settings(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr("face_ai.benchmark.runtime_metadata.platform.system", lambda: "TestOS")
    monkeypatch.setattr("face_ai.benchmark.runtime_metadata.platform.machine", lambda: "test-arch")
    monkeypatch.setattr("face_ai.benchmark.runtime_metadata.os.cpu_count", lambda: 4)
    versions = {
        "onnxruntime": "1", "insightface": "2", "numpy": "3",
        "opencv-python-headless": "4", "pillow": "5", "qdrant-client": "6",
    }
    monkeypatch.setattr(
        "face_ai.benchmark.runtime_metadata.metadata.version", lambda package: versions[package]
    )
    settings = Settings(
        insightface_model_root="/private/model-root",
        qdrant_url="http://private-qdrant:6333",
        insightface_detection_width=320,
        insightface_detection_height=240,
        insightface_detection_threshold=0.25,
    )

    result = asdict(collect_runtime_metadata(settings))

    assert result["operating_system"] == "TestOS"
    assert result["architecture"] == "test-arch"
    assert result["logical_cpu_count"] == 4
    assert result["detection_width"] == 320
    serialized = repr(result)
    assert "private" not in serialized
    assert "qdrant_url" not in result
    assert "model_root" not in result
