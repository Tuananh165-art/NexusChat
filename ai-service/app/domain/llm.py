from collections.abc import AsyncIterator
from typing import Literal

from pydantic import BaseModel, Field

LLMRole = Literal["system", "user", "assistant", "tool"]


class LLMMessage(BaseModel):
    role: LLMRole
    content: str


class LLMRequest(BaseModel):
    messages: list[LLMMessage]
    model: str | None = None
    temperature: float = Field(default=0.2, ge=0, le=2)
    max_tokens: int | None = Field(default=None, gt=0)
    stream: bool = False


class LLMUsage(BaseModel):
    prompt_tokens: int | None = None
    completion_tokens: int | None = None
    total_tokens: int | None = None


class LLMResponse(BaseModel):
    content: str
    model: str | None = None
    provider: str
    usage: LLMUsage | None = None
    raw_response_id: str | None = None


class LLMStreamEvent(BaseModel):
    event: str
    content: str | None = None
    done: bool = False


class LLMProvider:
    name: str

    async def complete(self, request: LLMRequest) -> LLMResponse:
        raise NotImplementedError

    async def stream(self, request: LLMRequest) -> AsyncIterator[LLMStreamEvent]:
        raise NotImplementedError
