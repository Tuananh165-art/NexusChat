from uuid import UUID

from pydantic import BaseModel, Field


class AgentCreateRequest(BaseModel):
    name: str = Field(min_length=1, max_length=120)
    description: str | None = Field(default=None, max_length=1000)
    system_prompt: str = Field(min_length=1, max_length=12000)
    model: str = Field(min_length=1, max_length=160)
    provider: str = Field(default="openai-compatible", max_length=80)
    temperature: float = Field(default=0.2, ge=0, le=2)
    enabled: bool = True


class AgentResponse(BaseModel):
    id: UUID
    name: str
    description: str | None = None
    system_prompt: str
    model: str
    provider: str
    temperature: float
    enabled: bool
