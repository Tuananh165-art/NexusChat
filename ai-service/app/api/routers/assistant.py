from collections.abc import AsyncIterator

from fastapi import APIRouter, HTTPException, status
from fastapi.responses import StreamingResponse

from app.api.dependencies import SettingsDep
from app.application.assistant import AssistantService
from app.domain.errors import ProviderConfigurationError, ProviderRequestError
from app.providers.factory import create_llm_provider
from app.schemas.assistant import RewriteRequest, RewriteResponse

router = APIRouter(prefix="/v1/assistant", tags=["assistant"])


@router.post("/rewrite", response_model=RewriteResponse)
async def rewrite(request: RewriteRequest, settings: SettingsDep) -> RewriteResponse:
    try:
        provider = create_llm_provider(settings)
        service = AssistantService(provider)
        return await service.rewrite(request)
    except ProviderConfigurationError as exc:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=str(exc),
        ) from exc
    except ProviderRequestError as exc:
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail=str(exc),
        ) from exc


@router.post("/rewrite/stream")
async def rewrite_stream(request: RewriteRequest, settings: SettingsDep) -> StreamingResponse:
    async def events() -> AsyncIterator[str]:
        try:
            provider = create_llm_provider(settings)
            service = AssistantService(provider)
            async for event in service.rewrite_stream(request):
                yield event.to_sse()
        except (ProviderConfigurationError, ProviderRequestError) as exc:
            from app.streaming.events import error_event

            yield error_event(str(exc)).to_sse()

    return StreamingResponse(events(), media_type="text/event-stream")
