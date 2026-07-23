from fastapi.testclient import TestClient

from app.api.app import create_app
from app.config.settings import Settings


def test_metrics_endpoint_returns_prometheus_payload() -> None:
    app = create_app(Settings(service_name="test-ai", environment="test"))
    client = TestClient(app)

    response = client.get("/metrics")

    assert response.status_code == 200
    assert "text/plain" in response.headers["content-type"]
    assert b"python_info" in response.content
