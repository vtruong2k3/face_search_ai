from __future__ import annotations

import math
import os
import platform
import sys
from dataclasses import dataclass
from importlib import metadata

from face_ai.settings import Settings


@dataclass(frozen=True, slots=True)
class RuntimeMetadata:
    operating_system: str
    architecture: str
    logical_cpu_count: int
    python_version: str
    onnx_provider: str
    onnxruntime_version: str
    insightface_version: str
    numpy_version: str
    opencv_version: str
    pillow_version: str
    qdrant_client_version: str
    insightface_pack: str
    detection_width: int
    detection_height: int
    detection_threshold: float
    execution_policy: str

    def __post_init__(self) -> None:
        text_values = (
            self.operating_system,
            self.architecture,
            self.python_version,
            self.onnx_provider,
            self.onnxruntime_version,
            self.insightface_version,
            self.numpy_version,
            self.opencv_version,
            self.pillow_version,
            self.qdrant_client_version,
            self.insightface_pack,
        )
        if any(not isinstance(value, str) or not value.strip() for value in text_values):
            raise ValueError("runtime metadata text values must be non-empty")
        if any(
            isinstance(value, bool) or not isinstance(value, int) or value <= 0
            for value in (self.logical_cpu_count, self.detection_width, self.detection_height)
        ):
            raise ValueError("runtime metadata counts must be positive integers")
        if (
            isinstance(self.detection_threshold, bool)
            or not isinstance(self.detection_threshold, (int, float))
            or not math.isfinite(self.detection_threshold)
            or not 0.0 <= self.detection_threshold <= 1.0
        ):
            raise ValueError("runtime metadata detection threshold must be between zero and one")
        if self.execution_policy != "serial":
            raise ValueError("runtime metadata execution policy must be serial")


def collect_runtime_metadata(settings: Settings) -> RuntimeMetadata:
    return RuntimeMetadata(
        operating_system=platform.system(),
        architecture=platform.machine(),
        logical_cpu_count=os.cpu_count() or 0,
        python_version=".".join(str(value) for value in sys.version_info[:3]),
        onnx_provider=settings.onnx_provider,
        onnxruntime_version=metadata.version("onnxruntime"),
        insightface_version=metadata.version("insightface"),
        numpy_version=metadata.version("numpy"),
        opencv_version=metadata.version("opencv-python-headless"),
        pillow_version=metadata.version("pillow"),
        qdrant_client_version=metadata.version("qdrant-client"),
        insightface_pack=settings.insightface_pack,
        detection_width=settings.insightface_detection_width,
        detection_height=settings.insightface_detection_height,
        detection_threshold=settings.insightface_detection_threshold,
        execution_policy="serial",
    )
