import json
from collections.abc import AsyncIterator
from typing import Any

import httpx

from app.config.settings import Settings
from app.domain.errors import ProviderConfigurationError, ProviderRequestError
from app.domain.llm import LLMRequest, LLMResponse, LLMStreamEvent, LLMUsage


class OpenAICompatibleProvider:
    name = "openai-compatible"

    def __init__(self, settings: Settings, client: httpx.AsyncClient | None = None) -> None:
        if settings.endpoint is None:
            raise ProviderConfigurationError("AI_ENDPOINT is required")
        if not settings.model:
            raise ProviderConfigurationError("AI_MODEL is required")

        self._settings = settings
        self._client = client or httpx.AsyncClient(timeout=settings.request_timeout_seconds)

    async def complete(self, request: LLMRequest) -> LLMResponse:
        payload = self._build_payload(request, stream=False)
        try:
            response = await self._client.post(
                self._chat_completions_url(),
                headers=self._headers(),
                json=payload,
            )
            response.raise_for_status()
        except httpx.HTTPError as exc:
            raise ProviderRequestError(f"provider request failed: {exc}") from exc

        data = response.json()
        return self._parse_completion(data)

    async def stream(self, request: LLMRequest) -> AsyncIterator[LLMStreamEvent]:
        payload = self._build_payload(request, stream=True)
        try:
            async with self._client.stream(
                "POST",
                self._chat_completions_url(),
                headers=self._headers(),
                json=payload,
            ) as response:
                response.raise_for_status()
                async for line in response.aiter_lines():
                    if not line.startswith("data: "):
                        continue
                    data = line.removeprefix("data: ").strip()
                    if data == "[DONE]":
                        yield LLMStreamEvent(event="final", done=True)
                        return
                    content = self._parse_stream_content(data)
                    if content:
                        yield LLMStreamEvent(event="token", content=content)
        except httpx.HTTPError as exc:
            raise ProviderRequestError(f"provider stream failed: {exc}") from exc

    def _build_payload(self, request: LLMRequest, stream: bool) -> dict[str, Any]:
        model = request.model or self._settings.model
        if model is None:
            raise ProviderConfigurationError("AI_MODEL is required")

        payload: dict[str, Any] = {
            "model": model,
            "messages": [message.model_dump() for message in request.messages],
            "temperature": request.temperature,
            "stream": stream,
        }
        if request.max_tokens is not None:
            payload["max_tokens"] = request.max_tokens
        return payload

    def _headers(self) -> dict[str, str]:
        headers = {"Content-Type": "application/json"}
        if self._settings.api_key:
            headers["Authorization"] = f"Bearer {self._settings.api_key}"
        return headers

    def _chat_completions_url(self) -> str:
        endpoint = str(self._settings.endpoint).rstrip("/")
        if endpoint.endswith("/chat/completions"):
            return endpoint
        return f"{endpoint}/chat/completions"

    def _parse_completion(self, data: dict[str, Any]) -> LLMResponse:
        choices = data.get("choices") or []
        content = ""
        if choices:
            message = choices[0].get("message") or {}
            content = message.get("content") or ""

        usage_data = data.get("usage")
        usage = LLMUsage(**usage_data) if isinstance(usage_data, dict) else None
        return LLMResponse(
            content=content,
            model=data.get("model"),
            provider=self.name,
            usage=usage,
            raw_response_id=data.get("id"),
        )

    def _parse_stream_content(self, data: str) -> str:
        try:
            parsed = json.loads(data)
        except json.JSONDecodeError:
            return data

        choices = parsed.get("choices") or []
        if not choices:
            return ""
        delta = choices[0].get("delta") or {}
        return delta.get("content") or ""
