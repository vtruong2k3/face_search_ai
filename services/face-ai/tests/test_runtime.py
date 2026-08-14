from __future__ import annotations

import hashlib
from pathlib import Path
from typing import Any

import pytest
from face_ai.runtime import ModelArtifact, ModelRuntime, runtime_status
from face_ai.settings import Settings


class FakeSession:
    def __init__(self, path: str, *, providers: list[str]) -> None:
        self.path = path
        self.providers = providers


class FakeSessionFactory:
    def __init__(self) -> None:
        self.created: list[FakeSession] = []

    def __call__(self, path: str, *, providers: list[str]) -> FakeSession:
        session = FakeSession(path, providers=providers)
        self.created.append(session)
        return session


def write_model(path: Path, content: bytes) -> str:
    path.write_bytes(content)
    return hashlib.sha256(content).hexdigest()


def test_settings_default_to_cpu_and_optional_models() -> None:
    settings = Settings(_env_file=None)

    assert settings.onnx_provider == "CPUExecutionProvider"
    assert settings.models_required is False
    assert settings.detector_model_path is None
    assert settings.embedder_model_path is None


def test_runtime_is_ready_when_optional_models_are_not_configured() -> None:
    runtime = ModelRuntime(
        settings=Settings(_env_file=None),
        available_providers=["CPUExecutionProvider"],
        session_factory=FakeSessionFactory(),
    )

    status = runtime.status()

    assert status["ready"] is True
    assert status["models"]["detector"]["state"] == "not_configured"
    assert status["models"]["embedder"]["state"] == "not_configured"


def test_runtime_is_not_ready_when_required_model_is_missing(tmp_path: Path) -> None:
    runtime = ModelRuntime(
        settings=Settings(
            _env_file=None,
            models_required=True,
            detector_model_path=tmp_path / "missing-detector.onnx",
            embedder_model_path=tmp_path / "missing-embedder.onnx",
        ),
        available_providers=["CPUExecutionProvider"],
        session_factory=FakeSessionFactory(),
    )

    status = runtime.status()

    assert status["ready"] is False
    assert status["models"]["detector"]["state"] == "missing"
    assert status["models"]["embedder"]["state"] == "missing"


def test_runtime_loads_models_and_reports_checksums_without_paths(tmp_path: Path) -> None:
    detector_path = tmp_path / "private" / "detector.onnx"
    detector_path.parent.mkdir()
    embedder_path = tmp_path / "private" / "embedder.onnx"
    detector_checksum = write_model(detector_path, b"detector-model")
    embedder_checksum = write_model(embedder_path, b"embedder-model")
    factory = FakeSessionFactory()
    runtime = ModelRuntime(
        settings=Settings(
            _env_file=None,
            models_required=True,
            detector_model_path=detector_path,
            detector_model_sha256=detector_checksum,
            embedder_model_path=embedder_path,
            embedder_model_sha256=embedder_checksum,
        ),
        available_providers=["CPUExecutionProvider"],
        session_factory=factory,
    )

    runtime.load()
    status = runtime.status()

    assert status["ready"] is True
    assert status["models"]["detector"] == {
        "state": "loaded",
        "sha256": detector_checksum,
    }
    assert status["models"]["embedder"] == {
        "state": "loaded",
        "sha256": embedder_checksum,
    }
    assert str(tmp_path) not in repr(status)
    assert [session.providers for session in factory.created] == [
        ["CPUExecutionProvider"],
        ["CPUExecutionProvider"],
    ]


def test_runtime_rejects_checksum_mismatch(tmp_path: Path) -> None:
    detector_path = tmp_path / "detector.onnx"
    write_model(detector_path, b"unexpected")
    runtime = ModelRuntime(
        settings=Settings(
            _env_file=None,
            detector_model_path=detector_path,
            detector_model_sha256="0" * 64,
        ),
        available_providers=["CPUExecutionProvider"],
        session_factory=FakeSessionFactory(),
    )

    with pytest.raises(RuntimeError, match="checksum"):
        runtime.load()

    assert runtime.status()["models"]["detector"]["state"] == "invalid"


def test_runtime_is_not_ready_when_provider_is_unavailable(tmp_path: Path) -> None:
    detector_path = tmp_path / "detector.onnx"
    write_model(detector_path, b"detector")
    runtime = ModelRuntime(
        settings=Settings(
            _env_file=None,
            detector_model_path=detector_path,
            onnx_provider="CUDAExecutionProvider",
        ),
        available_providers=["CPUExecutionProvider"],
        session_factory=FakeSessionFactory(),
    )

    with pytest.raises(RuntimeError, match="provider"):
        runtime.load()

    status = runtime.status()
    assert status["ready"] is False
    assert status["provider_ready"] is False


def test_model_artifact_requires_valid_sha256() -> None:
    with pytest.raises(ValueError, match="SHA-256"):
        ModelArtifact(name="detector", path=Path("model.onnx"), expected_sha256="invalid")


def test_runtime_status_keeps_health_contract(monkeypatch: pytest.MonkeyPatch) -> None:
    class StubRuntime:
        def status(self) -> dict[str, Any]:
            return {"ready": True, "provider": "CPUExecutionProvider"}

    monkeypatch.setattr("face_ai.runtime.get_model_runtime", lambda settings: StubRuntime())

    assert runtime_status(Settings(_env_file=None))["ready"] is True
