from pydantic import BaseModel, Field


class RewriteRequest(BaseModel):
    text: str = Field(min_length=1, max_length=8000)
    tone: str | None = Field(default=None, max_length=80)
    locale: str | None = Field(default=None, max_length=40)


class RewriteResponse(BaseModel):
    text: str
    provider: str
    model: str | None = None
