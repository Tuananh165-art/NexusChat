import httpx
from fastapi.testclient import TestClient

from app.api.app import create_app
from app.config.settings import Settings


def test_rewrite_returns_provider_response(monkeypatch) -> None:
    async def fake_post(self, url, headers=None, json=None):
        assert str(url).endswith("/chat/completions")
        assert json["messages"][0]["role"] == "system"
        return httpx.Response(
            status_code=200,
            request=httpx.Request("POST", url),
            json={
                "id": "chatcmpl-test",
                "model": "test-model",
                "choices": [{"message": {"content": "Hello, concise world."}}],
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
        "/v1/assistant/rewrite",
        json={"text": "hello world", "tone": "concise"},
    )

    assert response.status_code == 200
    assert response.json() == {
        "text": "Hello, concise world.",
        "provider": "openai-compatible",
        "model": "test-model",
    }


def test_rewrite_requires_provider_configuration(monkeypatch) -> None:
    monkeypatch.delenv("AI_ENDPOINT", raising=False)
    monkeypatch.delenv("AI_MODEL", raising=False)
    settings = Settings(service_name="test-ai", environment="test")
    settings.endpoint = None
    settings.model = None
    app = create_app(settings)
    client = TestClient(app)

    response = client.post("/v1/assistant/rewrite", json={"text": "hello"})

    assert response.status_code == 503
    assert "AI_ENDPOINT" in response.json()["detail"]
