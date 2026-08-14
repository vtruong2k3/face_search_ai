from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from typing import Literal, Protocol

from face_ai.benchmark.manifest import BenchmarkManifest, ManifestEntry
from face_ai.benchmark.metrics import Candidate, QueryObservation, QueryTimings
from face_ai.pipeline import PipelineResult
from face_ai.vector_store import VectorIndex, VectorRecord


class PipelinePort(Protocol):
    def process(self, content: bytes) -> PipelineResult: ...


@dataclass(frozen=True, slots=True)
class BenchmarkRun:
    observations: tuple[QueryObservation, ...]
    enrollment_failures: int


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
        observations: list[QueryObservation] = []
        subject_by_photo = {
            entry.image_id: entry.subject_id
            for entry in manifest.enrollment_entries
            if entry.subject_id is not None
        }
        self._index.create()
        try:
            records: list[VectorRecord] = []
            for entry in manifest.enrollment_entries:
                result = self._pipeline.process(self._load_bytes(entry))
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
            self._index.upsert(records)

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
                timings = QueryTimings(
                    decode_validation_ms=pipeline_timings.decode_validation_ms,
                    detection_ms=pipeline_timings.detection_ms,
                    alignment_ms=pipeline_timings.alignment_ms,
                    embedding_ms=pipeline_timings.embedding_ms,
                    vector_search_ms=vector_search_ms,
                    end_to_end_ms=elapsed,
                )
                observations.append(
                    QueryObservation(entry.image_id, entry.subject_id, candidates, status, timings)
                )
            return BenchmarkRun(tuple(observations), enrollment_failures)
        finally:
            self._index.teardown()
