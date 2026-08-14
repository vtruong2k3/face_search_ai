from __future__ import annotations

import argparse
import sys
from collections.abc import Sequence
from pathlib import Path

from face_ai.benchmark.manifest import BenchmarkManifest, ManifestError
from face_ai.benchmark.synthetic import run_synthetic


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="face-ai-benchmark")
    commands = parser.add_subparsers(dest="command", required=True)
    validate = commands.add_parser("validate")
    validate.add_argument("--manifest", type=Path, required=True)
    validate.add_argument("--dataset-root", type=Path, required=True)
    synthetic = commands.add_parser("synthetic")
    synthetic.add_argument("--output", type=Path, required=True)
    return parser


def main(argv: Sequence[str] | None = None) -> int:
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
    return 2


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
