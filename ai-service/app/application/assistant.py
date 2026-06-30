from collections.abc import AsyncIterator

from app.domain.llm import LLMProvider
from app.prompts.rewrite import build_rewrite_prompt
from app.schemas.assistant import RewriteRequest, RewriteResponse
from app.streaming.events import StreamEvent, final_event, status_event, token_event


class AssistantService:
    def __init__(self, provider: LLMProvider) -> None:
        self._provider = provider

    async def rewrite(self, request: RewriteRequest) -> RewriteResponse:
        llm_request = build_rewrite_prompt(
            text=request.text,
            tone=request.tone,
            locale=request.locale,
        )
        response = await self._provider.complete(llm_request)
        return RewriteResponse(
            text=response.content,
            provider=response.provider,
            model=response.model,
        )

    async def rewrite_stream(self, request: RewriteRequest) -> AsyncIterator[StreamEvent]:
        llm_request = build_rewrite_prompt(
            text=request.text,
            tone=request.tone,
            locale=request.locale,
        )
        yield status_event("thinking")
        async for event in self._provider.stream(llm_request):
            if event.event == "token" and event.content:
                yield token_event(event.content)
            if event.done:
                yield final_event()
