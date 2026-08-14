import importlib

from face_ai.main import app, create_app
from face_ai.settings import Settings
from fastapi.testclient import TestClient


def test_health() -> None:
    response = TestClient(app).get("/health/ready")
    assert response.status_code == 200
    payload = response.json()
    assert payload["status"] == "ready"
    assert payload["service"] == "face-ai"
    assert payload["runtime"]["provider"] == "CPUExecutionProvider"
    assert payload["runtime"]["model_configured"] is False


def test_unavailable_enabled_pipeline_only_fails_readiness(monkeypatch) -> None:
    private_root = "/private/models/not-for-output"
    main_module = importlib.import_module("face_ai.main")
    monkeypatch.setattr(
        main_module,
        "get_settings",
        lambda: Settings(
            _env_file=None,
            insightface_enabled=True,
            insightface_model_root=private_root,
        ),
    )
    client = TestClient(create_app())

    live = client.get("/health/live")
    ready = client.get("/health/ready")

    assert live.status_code == 200
    assert ready.status_code == 503
    payload = ready.json()
    assert payload["status"] == "not_ready"
    assert payload["runtime"]["pipeline"]["state"] == "load_failed"
    assert private_root not in ready.text
