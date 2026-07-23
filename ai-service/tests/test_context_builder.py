import httpx
import pytest

from app.config.settings import Settings
from app.context.builder import ConversationContextBuilder
from app.context.chat_client import ChatContextClient


@pytest.mark.asyncio
async def test_chat_context_client_fetches_messages_with_bearer_token() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.headers["Authorization"] == "Bearer channel-token"
        assert request.url.path == "/api/chat/channel/messages"
        return httpx.Response(
            200,
            json={
                "messages": [
                    {
                        "message_id": "1",
                        "event": 0,
                        "user_id": "42",
                        "payload": "hello",
                        "time": 123,
                    }
                ],
                "next_ps": "next",
            },
        )

    client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    settings = Settings(
        service_name="test-ai",
        environment="test",
        chat_service_base_url="http://chat.local/api/chat",
    )
    chat_client = ChatContextClient(settings, client=client)

    page = await chat_client.list_channel_messages(access_token="channel-token")

    assert page.next_page_state == "next"
    assert page.messages[0].payload == "hello"


@pytest.mark.asyncio
async def test_conversation_context_builder_limits_messages() -> None:
    def handler(_: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "messages": [
                    {"message_id": str(i), "event": 0, "user_id": "u", "payload": f"m{i}"}
                    for i in range(5)
                ]
            },
        )

    client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    settings = Settings(
        service_name="test-ai",
        environment="test",
        chat_service_base_url="http://chat.local/api/chat",
    )
    builder = ConversationContextBuilder(ChatContextClient(settings, client=client))

    context = await builder.build_from_channel(access_token="token", max_messages=2)

    assert [message.payload for message in context.messages] == ["m3", "m4"]
    assert context.render_for_prompt() == "user:u message:m3\nuser:u message:m4"
