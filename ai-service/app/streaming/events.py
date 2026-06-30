import json
from typing import Any

from pydantic import BaseModel


class StreamEvent(BaseModel):
    event: str
    data: dict[str, Any]

    def to_sse(self) -> str:
        return f"event: {self.event}\ndata: {json.dumps(self.data, ensure_ascii=False)}\n\n"


def token_event(content: str) -> StreamEvent:
    return StreamEvent(event="token", data={"content": content})


def status_event(status: str) -> StreamEvent:
    return StreamEvent(event="status", data={"status": status})


def final_event(content: str | None = None) -> StreamEvent:
    data: dict[str, Any] = {"done": True}
    if content is not None:
        data["content"] = content
    return StreamEvent(event="final", data=data)


def error_event(message: str) -> StreamEvent:
    return StreamEvent(event="error", data={"message": message})
