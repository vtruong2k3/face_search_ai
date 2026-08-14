from __future__ import annotations

import hashlib
from collections.abc import Sequence
from dataclasses import dataclass
from functools import lru_cache
from pathlib import Path
from typing import Any, Protocol

import cv2
import numpy
import onnxruntime  # type: ignore[import-untyped]
from PIL import Image

from face_ai.settings import Settings


class SessionFactory(Protocol):
    def __call__(self, path: str, *, providers: list[str]) -> Any: ...


@dataclass(frozen=True, slots=True)
class ModelArtifact:
    name: str
    path: Path
    expected_sha256: str | None = None

    def __post_init__(self) -> None:
        checksum = self.expected_sha256
        if checksum is not None and (
            len(checksum) != 64 or any(character not in "0123456789abcdefABCDEF" for character in checksum)
        ):
            raise ValueError("expected checksum must be a hexadecimal SHA-256 value")


class ModelRuntime:
    def __init__(
        self,
        *,
        settings: Settings,
        available_providers: Sequence[str] | None = None,
        session_factory: SessionFactory | None = None,
    ) -> None:
        self._settings = settings
        self._available_providers = tuple(available_providers or onnxruntime.get_available_providers())
        self._session_factory = session_factory or onnxruntime.InferenceSession
        self._sessions: dict[str, Any] = {}
        self._states: dict[str, str] = {}
        self._checksums: dict[str, str] = {}
        self._load_error: str | None = None

    def load(self) -> None:
        self._load_error = None
        if self._settings.onnx_provider not in self._available_providers:
            self._load_error = "configured ONNX provider is unavailable"
            raise RuntimeError(self._load_error)

        for artifact in self._artifacts():
            if not artifact.path.is_file():
                self._states[artifact.name] = "missing"
                if self._settings.models_required:
                    self._load_error = f"required {artifact.name} model is missing"
                    raise RuntimeError(self._load_error)
                continue

            checksum = _sha256(artifact.path)
            if artifact.expected_sha256 and checksum != artifact.expected_sha256.lower():
                self._states[artifact.name] = "invalid"
                self._load_error = f"{artifact.name} model checksum mismatch"
                raise RuntimeError(self._load_error)

            try:
                session = self._session_factory(
                    str(artifact.path),
                    providers=[self._settings.onnx_provider],
                )
            except Exception as error:
                self._states[artifact.name] = "load_failed"
                self._load_error = f"{artifact.name} model failed to load"
                raise RuntimeError(self._load_error) from error

            self._sessions[artifact.name] = session
            self._checksums[artifact.name] = checksum
            self._states[artifact.name] = "loaded"

    def session(self, name: str) -> Any:
        try:
            return self._sessions[name]
        except KeyError as error:
            raise RuntimeError(f"{name} model is not loaded") from error

    def status(self) -> dict[str, object]:
        provider_ready = self._settings.onnx_provider in self._available_providers
        model_status = {
            artifact.name: self._model_status(artifact)
            for artifact in self._artifacts(include_unconfigured=True)
        }
        configured_states = [model["state"] for model in model_status.values()]
        required_models_ready = not self._settings.models_required or all(
            state == "loaded" for state in configured_states
        )
        configured_models_valid = all(
            state in {"loaded", "not_configured"} for state in configured_states
        )
        ready = provider_ready and required_models_ready and configured_models_valid and self._load_error is None
        return {
            "ready": ready,
            "provider": self._settings.onnx_provider,
            "provider_ready": provider_ready,
            "available_providers": list(self._available_providers),
            "models_required": self._settings.models_required,
            "model_configured": any(state != "not_configured" for state in configured_states),
            "models": model_status,
            "opencv_version": cv2.__version__,
            "numpy_version": numpy.__version__,
            "pillow_version": Image.__version__,
            "error": self._load_error,
        }

    def _artifacts(self, *, include_unconfigured: bool = False) -> list[ModelArtifact]:
        configured = [
            ("detector", self._settings.detector_model_path, self._settings.detector_model_sha256),
            ("embedder", self._settings.embedder_model_path, self._settings.embedder_model_sha256),
        ]
        artifacts: list[ModelArtifact] = []
        for name, path, checksum in configured:
            if path is not None:
                artifacts.append(ModelArtifact(name=name, path=path, expected_sha256=checksum))
            elif include_unconfigured:
                artifacts.append(ModelArtifact(name=name, path=Path()))
        return artifacts

    def _model_status(self, artifact: ModelArtifact) -> dict[str, str]:
        if artifact.name in self._states:
            state = self._states[artifact.name]
        elif (artifact.name == "detector" and self._settings.detector_model_path is None) or (
            artifact.name == "embedder" and self._settings.embedder_model_path is None
        ):
            state = "not_configured"
        elif artifact.path.is_file():
            state = "configured"
        else:
            state = "missing"

        status = {"state": state}
        checksum = self._checksums.get(artifact.name)
        if checksum is not None:
            status["sha256"] = checksum
        return status


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as model_file:
        for chunk in iter(lambda: model_file.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


@dataclass(frozen=True, slots=True)
class RuntimeKey:
    models_required: bool
    detector_model_path: Path | None
    detector_model_sha256: str | None
    embedder_model_path: Path | None
    embedder_model_sha256: str | None
    onnx_provider: str
    qdrant_url: str


@lru_cache
def _cached_runtime(settings_key: RuntimeKey) -> ModelRuntime:
    settings = Settings(
        models_required=settings_key.models_required,
        detector_model_path=settings_key.detector_model_path,
        detector_model_sha256=settings_key.detector_model_sha256,
        embedder_model_path=settings_key.embedder_model_path,
        embedder_model_sha256=settings_key.embedder_model_sha256,
        onnx_provider=settings_key.onnx_provider,
        qdrant_url=settings_key.qdrant_url,
    )
    return ModelRuntime(settings=settings)


def get_model_runtime(settings: Settings) -> ModelRuntime:
    key = RuntimeKey(
        models_required=settings.models_required,
        detector_model_path=settings.detector_model_path,
        detector_model_sha256=settings.detector_model_sha256,
        embedder_model_path=settings.embedder_model_path,
        embedder_model_sha256=settings.embedder_model_sha256,
        onnx_provider=settings.onnx_provider,
        qdrant_url=settings.qdrant_url,
    )
    return _cached_runtime(key)


def runtime_status(settings: Settings) -> dict[str, object]:
    return get_model_runtime(settings).status()
