from photo_worker.jobs import JobEnvelope


def test_job_envelope_round_trip() -> None:
    job = JobEnvelope(job_id="smoke-1", type="connectivity.smoke")
    parsed = JobEnvelope.from_stream({b"job": job.to_json().encode()})
    assert parsed == job
