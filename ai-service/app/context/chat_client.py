from typing import Any

import httpx
from pydantic import BaseModel, Field

from app.config.settings import Settings
from app.domain.errors import AIServiceError


class ChatContextConfigurationError(AIServiceError):
    """Raised when Chat service context access is not configured."""


class ChatMessageContext(BaseModel):
    message_id: str | None = None
    event: int
    user_id: str
    payload: str
    time: int | str | None = None
    parent_id: str | None = None


class ChatMessagesPage(BaseModel):
    messages: list[ChatMessageContext] = Field(default_factory=list)
    next_page_state: str | None = None


class ChatContextClient:
    def __init__(self, settings: Settings, client: httpx.AsyncClient | None = None) -> None:
        if settings.chat_service_base_url is None:
            raise ChatContextConfigurationError("CHAT_SERVICE_BASE_URL is required")
        self._base_url = str(settings.chat_service_base_url).rstrip("/")
        self._client = client or httpx.AsyncClient(timeout=5.0)

    async def list_channel_messages(
        self,
        access_token: str,
        page_state: str | None = None,
    ) -> ChatMessagesPage:
        params: dict[str, str] = {}
        if page_state:
            params["ps"] = page_state

        response = await self._client.get(
            f"{self._base_url}/channel/messages",
            params=params,
            headers={"Authorization": f"Bearer {access_token}"},
        )
        response.raise_for_status()
        return self._parse_messages_page(response.json())

    def _parse_messages_page(self, data: dict[str, Any]) -> ChatMessagesPage:
        return ChatMessagesPage(
            messages=[ChatMessageContext(**item) for item in data.get("messages", [])],
            next_page_state=data.get("next_ps") or data.get("nextPageState"),
        )
