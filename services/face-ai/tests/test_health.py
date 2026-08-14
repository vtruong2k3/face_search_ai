from face_ai.main import app
from fastapi.testclient import TestClient


def test_health() -> None:
    response = TestClient(app).get("/health/ready")
    assert response.status_code == 200
    payload = response.json()
    assert payload["status"] == "ready"
    assert payload["service"] == "face-ai"
    assert payload["runtime"]["provider"] == "CPUExecutionProvider"
    assert payload["runtime"]["model_configured"] is False
