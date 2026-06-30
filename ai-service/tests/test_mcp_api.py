from fastapi.testclient import TestClient

from app.api.app import create_app
from app.config.settings import Settings


def test_list_mcp_tools_returns_preview_only_tools() -> None:
    app = create_app(Settings(service_name="test-ai", environment="test"))
    client = TestClient(app)

    response = client.get("/v1/mcp/tools")

    assert response.status_code == 200
    tools = response.json()
    assert {tool["name"] for tool in tools} == {"github_issue", "calendar_event"}
    assert all(tool["requires_approval"] for tool in tools)


def test_preview_mcp_tool_never_executes() -> None:
    app = create_app(Settings(service_name="test-ai", environment="test"))
    client = TestClient(app)

    response = client.post(
        "/v1/mcp/tools/preview",
        json={"tool_name": "github_issue", "arguments": {"title": "Ship AI"}},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["status"] == "preview"
    assert body["requires_approval"] is True
    assert body["preview"]["will_execute"] is False
