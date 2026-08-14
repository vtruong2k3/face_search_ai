from __future__ import annotations

import argparse
import sys
import time
from collections.abc import Callable, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any, cast

from qdrant_client import QdrantClient

from face_ai.benchmark.calibration import CalibrationPolicy
from face_ai.benchmark.execution import execute_benchmark
from face_ai.benchmark.manifest import BenchmarkManifest, ManifestError
from face_ai.benchmark.synthetic import run_synthetic
from face_ai.pipeline import FacePipeline
from face_ai.qdrant_store import BenchmarkQdrantIndex, QdrantClientPort
from face_ai.runtime import get_insightface_pipeline_runtime, verify_buffalo_l_checksums
from face_ai.settings import get_settings
from face_ai.vector_store import VectorCollection, VectorDistance, VectorIndex


@dataclass(frozen=True, slots=True)
class RunDependencies:
    verify_model: Callable[[BenchmarkManifest], None]
    get_pipeline: Callable[[], FacePipeline | Any | None]
    create_index: Callable[[BenchmarkManifest], VectorIndex | Any]
    execute: Callable[..., dict[str, Any]]
    clock_ms: Callable[[], float]


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="face-ai-benchmark")
    commands = parser.add_subparsers(dest="command", required=True)
    validate = commands.add_parser("validate")
    validate.add_argument("--manifest", type=Path, required=True)
    validate.add_argument("--dataset-root", type=Path, required=True)
    synthetic = commands.add_parser("synthetic")
    synthetic.add_argument("--output", type=Path, required=True)
    run = commands.add_parser("run")
    run.add_argument("--manifest", type=Path, required=True)
    run.add_argument("--dataset-root", type=Path, required=True)
    run.add_argument("--output", type=Path, required=True)
    run.add_argument("--max-far", type=float, required=True)
    run.add_argument("--min-recall", type=float, required=True)
    run.add_argument("--max-frr", type=float)
    return parser


def main(
    argv: Sequence[str] | None = None,
    *,
    run_dependencies: RunDependencies | None = None,
) -> int:
    args = _parser().parse_args(argv)
    if args.command == "validate":
        return _validate(args.manifest, args.dataset_root)
    if args.command == "synthetic":
        try:
            run_synthetic(args.output)
        except (OSError, ValueError, RuntimeError):
            print("synthetic benchmark failed", file=sys.stderr)
            return 2
        print("synthetic benchmark passed")
        return 0
    if args.command == "run":
        return _run(args, run_dependencies or _production_dependencies())
    return 2


def _run(args: argparse.Namespace, dependencies: RunDependencies) -> int:
    try:
        policy = CalibrationPolicy(
            max_far=args.max_far,
            min_recall=args.min_recall,
            max_frr=args.max_frr,
        )
        manifest = BenchmarkManifest.load(args.manifest)
        dependencies.verify_model(manifest)
        pipeline = dependencies.get_pipeline()
        if pipeline is None:
            raise RuntimeError("pipeline is unavailable")
        index = dependencies.create_index(manifest)
        dependencies.execute(
            manifest=manifest,
            dataset_root=args.dataset_root,
            output=args.output,
            pipeline=pipeline,
            index=index,
            clock_ms=dependencies.clock_ms,
            policy=policy,
        )
    except Exception:  # noqa: BLE001 -- CLI must sanitize external/model/storage failures
        print("benchmark run failed", file=sys.stderr)
        return 2
    print("benchmark run completed")
    return 0


def _production_dependencies() -> RunDependencies:
    settings = get_settings()

    def verify_model(manifest: BenchmarkManifest) -> None:
        model_root = settings.insightface_model_root
        if not settings.insightface_enabled or model_root is None:
            raise RuntimeError("pipeline is unavailable")
        verify_buffalo_l_checksums(
            model_root=model_root,
            detector_sha256=manifest.model.detector_sha256,
            embedder_sha256=manifest.model.embedder_sha256,
        )

    def get_pipeline() -> FacePipeline | None:
        return get_insightface_pipeline_runtime(settings).pipeline()

    def create_index(manifest: BenchmarkManifest) -> BenchmarkQdrantIndex:
        collection_name = f"benchmark-{manifest.benchmark_id}-{manifest.fingerprint[:12]}"
        collection = VectorCollection(collection_name, 512, VectorDistance.COSINE)
        return BenchmarkQdrantIndex(
            client=cast(QdrantClientPort, QdrantClient(url=settings.qdrant_url)),
            collection=collection,
        )

    return RunDependencies(
        verify_model=verify_model,
        get_pipeline=get_pipeline,
        create_index=create_index,
        execute=execute_benchmark,
        clock_ms=lambda: time.perf_counter_ns() / 1_000_000,
    )


def _validate(manifest_path: Path, dataset_root: Path) -> int:
    try:
        manifest = BenchmarkManifest.load(manifest_path)
        for entry in manifest.entries:
            manifest.resolve_image(dataset_root, entry)
    except (ManifestError, OSError):
        print("benchmark validation failed", file=sys.stderr)
        return 2
    print(
        f"benchmark={manifest.benchmark_id} fingerprint={manifest.fingerprint} "
        f"enrollment={len(manifest.enrollment_entries)} queries={len(manifest.query_entries)}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
