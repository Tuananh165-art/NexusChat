from fastapi.testclient import TestClient

from app.api.app import create_app
from app.config.settings import Settings


def test_create_and_get_agent() -> None:
    app = create_app(Settings(service_name="test-ai", environment="test"))
    client = TestClient(app)

    create_response = client.post(
        "/v1/agents",
        json={
            "name": "Support Agent",
            "system_prompt": "Help users.",
            "model": "test-model",
        },
    )

    assert create_response.status_code == 201
    agent = create_response.json()
    assert agent["name"] == "Support Agent"

    get_response = client.get(f"/v1/agents/{agent['id']}")
    assert get_response.status_code == 200
    assert get_response.json()["id"] == agent["id"]


def test_list_agents() -> None:
    app = create_app(Settings(service_name="test-ai", environment="test"))
    client = TestClient(app)

    client.post(
        "/v1/agents",
        json={"name": "A", "system_prompt": "Prompt", "model": "model"},
    )

    response = client.get("/v1/agents")

    assert response.status_code == 200
    assert len(response.json()) == 1
