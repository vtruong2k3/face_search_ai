from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from typing import Literal, Protocol

from face_ai.benchmark.execution_policy import BenchmarkExecution
from face_ai.benchmark.manifest import BenchmarkManifest, ManifestEntry
from face_ai.benchmark.metrics import (
    Candidate,
    EnrollmentTiming,
    QueryObservation,
    QueryTimings,
    VectorIndexTimings,
)
from face_ai.pipeline import PipelineResult
from face_ai.vector_store import VectorIndex, VectorRecord


class PipelinePort(Protocol):
    def process(self, content: bytes) -> PipelineResult: ...


@dataclass(frozen=True, slots=True)
class BenchmarkRun:
    observations: tuple[QueryObservation, ...]
    enrollment_failures: int
    execution: BenchmarkExecution
    enrollment_timings: tuple[EnrollmentTiming, ...]
    indexed_vector_count: int
    vector_index_timings: VectorIndexTimings


BENCHMARK_UPSERT_BATCH_SIZE = 100


class BenchmarkRunner:
    def __init__(
        self,
        *,
        pipeline: PipelinePort,
        index: VectorIndex,
        load_bytes: Callable[[ManifestEntry], bytes],
        clock_ms: Callable[[], float],
    ) -> None:
        self._pipeline = pipeline
        self._index = index
        self._load_bytes = load_bytes
        self._clock_ms = clock_ms

    def run(self, manifest: BenchmarkManifest) -> BenchmarkRun:
        enrollment_failures = 0
        enrollment_timings: list[EnrollmentTiming] = []
        observations: list[QueryObservation] = []
        subject_by_photo = {
            entry.image_id: entry.subject_id
            for entry in manifest.enrollment_entries
            if entry.subject_id is not None
        }
        setup_started = self._clock_ms()
        try:
            self._index.create()
            setup_ms = self._clock_ms() - setup_started
            records: list[VectorRecord] = []
            for entry in manifest.enrollment_entries:
                enrollment_started = self._clock_ms()
                result = self._pipeline.process(self._load_bytes(entry))
                enrollment_elapsed = self._clock_ms() - enrollment_started
                pipeline_timings = result.timings
                enrollment_timings.append(
                    EnrollmentTiming(
                        pipeline_timings.decode_validation_ms,
                        pipeline_timings.detection_ms,
                        pipeline_timings.alignment_ms,
                        pipeline_timings.embedding_ms,
                        enrollment_elapsed,
                    )
                )
                if len(result.faces) != 1 or entry.subject_id is None:
                    enrollment_failures += 1
                    continue
                records.append(
                    VectorRecord(
                        face_id=f"{entry.image_id}:0",
                        photo_id=entry.image_id,
                        dataset_id=manifest.dataset.id,
                        event_id=manifest.dataset.event_id,
                        embedding=result.faces[0].embedding,
                    )
                )
            upsert_started = self._clock_ms()
            self._index.upsert(records, batch_size=BENCHMARK_UPSERT_BATCH_SIZE)
            upsert_ms = self._clock_ms() - upsert_started

            for entry in manifest.query_entries:
                started = self._clock_ms()
                result = self._pipeline.process(self._load_bytes(entry))
                status: Literal["ok", "no_face", "ambiguous"]
                vector_search_ms: float | None = None
                if len(result.faces) == 0:
                    status = "no_face"
                    candidates: tuple[Candidate, ...] = ()
                elif len(result.faces) > 1:
                    status = "ambiguous"
                    candidates = ()
                else:
                    status = "ok"
                    search_started = self._clock_ms()
                    found = self._index.search(
                        result.faces[0].embedding,
                        dataset_id=manifest.dataset.id,
                        event_id=manifest.dataset.event_id,
                        limit=manifest.search.limit,
                        score_threshold=None,
                    )
                    vector_search_ms = self._clock_ms() - search_started
                    candidates = tuple(
                        Candidate(subject, item.score)
                        for item in found
                        if (subject := subject_by_photo.get(item.photo_id)) is not None
                    )
                elapsed = self._clock_ms() - started
                pipeline_timings = result.timings
                observations.append(
                    QueryObservation(
                        entry.image_id,
                        entry.subject_id,
                        candidates,
                        status,
                        QueryTimings(
                            pipeline_timings.decode_validation_ms,
                            pipeline_timings.detection_ms,
                            pipeline_timings.alignment_ms,
                            pipeline_timings.embedding_ms,
                            vector_search_ms,
                            elapsed,
                        ),
                    )
                )
        finally:
            teardown_started = self._clock_ms()
            try:
                self._index.teardown()
            finally:
                teardown_ms = self._clock_ms() - teardown_started

        return BenchmarkRun(
            tuple(observations),
            enrollment_failures,
            BenchmarkExecution.enrollment_primed(
                warmup_inference_count=len(manifest.enrollment_entries)
            ),
            tuple(enrollment_timings),
            len(records),
            VectorIndexTimings(
                setup_ms,
                upsert_ms,
                teardown_ms,
                BENCHMARK_UPSERT_BATCH_SIZE,
            ),
        )
