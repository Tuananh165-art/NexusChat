from pydantic import BaseModel, Field

from app.context.chat_client import ChatContextClient, ChatMessageContext


class ConversationContext(BaseModel):
    channel_id: str | None = None
    messages: list[ChatMessageContext] = Field(default_factory=list)

    def render_for_prompt(self) -> str:
        lines: list[str] = []
        for message in self.messages:
            lines.append(f"user:{message.user_id} message:{message.payload}")
        return "\n".join(lines)


class ConversationContextBuilder:
    def __init__(self, chat_client: ChatContextClient) -> None:
        self._chat_client = chat_client

    async def build_from_channel(
        self,
        access_token: str,
        channel_id: str | None = None,
        max_messages: int = 50,
    ) -> ConversationContext:
        page = await self._chat_client.list_channel_messages(access_token=access_token)
        messages = page.messages[-max_messages:]
        return ConversationContext(channel_id=channel_id, messages=messages)
