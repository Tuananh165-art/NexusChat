import httpx
from fastapi.testclient import TestClient

from app.api.app import create_app
from app.config.settings import Settings


def test_workflow_draft_requires_provider_configuration(monkeypatch) -> None:
    monkeypatch.delenv("AI_ENDPOINT", raising=False)
    monkeypatch.delenv("AI_MODEL", raising=False)
    settings = Settings(service_name="test-ai", environment="test")
    settings.endpoint = None
    settings.model = None
    app = create_app(settings)
    client = TestClient(app)

    response = client.post(
        "/v1/workflows/draft",
        json={"workflow_type": "tasks", "source_text": "Ship the AI service."},
    )

    assert response.status_code == 503


def test_workflow_draft_returns_preview(monkeypatch) -> None:
    async def fake_post(self, url, headers=None, json=None):
        return httpx.Response(
            status_code=200,
            request=httpx.Request("POST", url),
            json={
                "id": "chatcmpl-test",
                "model": "test-model",
                "choices": [{"message": {"content": '{"tasks": ["Ship the AI service"]}'}}],
            },
        )

    monkeypatch.setattr(httpx.AsyncClient, "post", fake_post)
    app = create_app(
        Settings(
            service_name="test-ai",
            environment="test",
            endpoint="https://provider.example/v1",
            api_key="secret",
            model="test-model",
        )
    )
    client = TestClient(app)

    response = client.post(
        "/v1/workflows/draft",
        json={"workflow_type": "tasks", "source_text": "Ship the AI service."},
    )

    assert response.status_code == 200
    assert response.json()["status"] == "preview"
    assert response.json()["execution_required_approval"] is True
    assert response.json()["preview"] == {"tasks": ["Ship the AI service"]}
