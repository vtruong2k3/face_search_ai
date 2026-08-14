from __future__ import annotations

import hashlib
from pathlib import Path
from typing import Any

import pytest
from face_ai.models.insightface import InsightFaceAdapters
from face_ai.pipeline import FacePipeline
from face_ai.runtime import (
    InsightFacePipelineRuntime,
    ModelArtifact,
    ModelRuntime,
    runtime_status,
    verify_buffalo_l_checksums,
)
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


def test_verify_buffalo_l_checksums_matches_canonical_artifacts(tmp_path: Path) -> None:
    pack = tmp_path / "models" / "buffalo_l"
    pack.mkdir(parents=True)
    detector_sha256 = write_model(pack / "det_10g.onnx", b"detector")
    embedder_sha256 = write_model(pack / "w600k_r50.onnx", b"embedder")

    verify_buffalo_l_checksums(
        model_root=tmp_path,
        detector_sha256=detector_sha256.upper(),
        embedder_sha256=embedder_sha256.upper(),
    )


@pytest.mark.parametrize(
    ("detector_sha256", "embedder_sha256"),
    [("0" * 64, None), (None, "0" * 64)],
)
def test_verify_buffalo_l_checksums_rejects_mismatch(
    tmp_path: Path, detector_sha256: str | None, embedder_sha256: str | None
) -> None:
    pack = tmp_path / "models" / "buffalo_l"
    pack.mkdir(parents=True)
    actual_detector = write_model(pack / "det_10g.onnx", b"detector")
    actual_embedder = write_model(pack / "w600k_r50.onnx", b"embedder")

    with pytest.raises(RuntimeError, match="model artifact verification failed"):
        verify_buffalo_l_checksums(
            model_root=tmp_path,
            detector_sha256=detector_sha256 or actual_detector,
            embedder_sha256=embedder_sha256 or actual_embedder,
        )


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


def test_insightface_pipeline_is_disabled_without_preparation() -> None:
    def fail_prepare(**kwargs: Any) -> InsightFaceAdapters:
        raise AssertionError("disabled pipeline must not be prepared")

    runtime = InsightFacePipelineRuntime(
        settings=Settings(_env_file=None),
        prepare=fail_prepare,
    )

    assert runtime.pipeline() is None
    assert runtime.status() == {
        "enabled": False,
        "backend": "insightface",
        "pack": "buffalo_l",
        "state": "disabled",
        "ready": True,
    }


def test_insightface_pipeline_requires_external_root() -> None:
    runtime = InsightFacePipelineRuntime(
        settings=Settings(_env_file=None, insightface_enabled=True),
    )

    assert runtime.pipeline() is None
    assert runtime.status()["state"] == "not_configured"
    assert runtime.status()["ready"] is False


def test_insightface_pipeline_prepares_once_and_sanitizes_failures(tmp_path: Path) -> None:
    model_root = tmp_path / "private-model-root"
    calls: list[dict[str, Any]] = []

    class FakeDetector:
        def detect(self, image: Any) -> list[Any]:
            return []

    class FakeAligner:
        def align(self, image: Any, face: Any) -> Any:
            return image

    class FakeEmbedder:
        dimension = 512

        def embed(self, aligned_face: Any) -> Any:
            return aligned_face

    adapters = InsightFaceAdapters(
        detector=FakeDetector(),  # type: ignore[arg-type]
        aligner=FakeAligner(),  # type: ignore[arg-type]
        embedder=FakeEmbedder(),  # type: ignore[arg-type]
    )

    def prepare(**kwargs: Any) -> InsightFaceAdapters:
        calls.append(kwargs)
        return adapters

    settings = Settings(
        _env_file=None,
        insightface_enabled=True,
        insightface_model_root=model_root,
        insightface_detection_width=320,
        insightface_detection_height=480,
        insightface_detection_threshold=0.75,
    )
    runtime = InsightFacePipelineRuntime(settings=settings, prepare=prepare)

    first = runtime.pipeline()
    second = runtime.pipeline()

    assert isinstance(first, FacePipeline)
    assert second is first
    assert calls == [
        {
            "model_root": model_root,
            "detection_size": (320, 480),
            "detection_threshold": 0.75,
        }
    ]
    assert runtime.status()["state"] == "ready"

    def fail_prepare(**kwargs: Any) -> InsightFaceAdapters:
        raise RuntimeError(f"failed at {model_root}/models/buffalo_l/model.onnx")

    failed = InsightFacePipelineRuntime(settings=settings, prepare=fail_prepare)
    assert failed.pipeline() is None
    failed_status = failed.status()
    assert failed_status["state"] == "load_failed"
    assert failed_status["error"] == "pipeline initialization failed"
    assert str(model_root) not in repr(failed_status)


def test_insightface_pipeline_rejects_non_cpu_provider_before_preparation(tmp_path: Path) -> None:
    def fail_prepare(**kwargs: Any) -> InsightFaceAdapters:
        raise AssertionError("unsupported provider must not prepare pipeline")

    runtime = InsightFacePipelineRuntime(
        settings=Settings(
            _env_file=None,
            insightface_enabled=True,
            insightface_model_root=tmp_path,
            onnx_provider="CUDAExecutionProvider",
        ),
        prepare=fail_prepare,
    )

    assert runtime.pipeline() is None
    assert runtime.status()["state"] == "unsupported_provider"


def test_runtime_status_combines_legacy_and_pipeline_readiness(monkeypatch: pytest.MonkeyPatch) -> None:
    class StubRuntime:
        def status(self) -> dict[str, Any]:
            return {"ready": True, "provider": "CPUExecutionProvider"}

    class StubPipelineRuntime:
        def status(self) -> dict[str, Any]:
            return {"enabled": True, "state": "load_failed", "ready": False}

    monkeypatch.setattr("face_ai.runtime.get_model_runtime", lambda settings: StubRuntime())
    monkeypatch.setattr(
        "face_ai.runtime.get_insightface_pipeline_runtime",
        lambda settings: StubPipelineRuntime(),
    )

    status = runtime_status(Settings(_env_file=None))
    assert status["ready"] is False
    assert status["pipeline"]["state"] == "load_failed"
