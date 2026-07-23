from fastapi.testclient import TestClient

from app.api.app import create_app
from app.config.settings import Settings


def test_health_returns_service_status() -> None:
    app = create_app(Settings(service_name="test-ai", environment="test"))
    client = TestClient(app)

    response = client.get("/health")

    assert response.status_code == 200
    assert response.json() == {
        "status": "ok",
        "service": "test-ai",
        "environment": "test",
    }


def test_ready_returns_readiness_status() -> None:
    app = create_app(Settings(service_name="test-ai", environment="test"))
    client = TestClient(app)

    response = client.get("/ready")

    assert response.status_code == 200
    assert response.json()["status"] == "ready"
