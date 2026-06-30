from fastapi import APIRouter, HTTPException, status

from app.api.dependencies import SettingsDep
from app.domain.errors import ProviderConfigurationError, ProviderRequestError
from app.providers.factory import create_llm_provider
from app.schemas.workflow import WorkflowDraftRequest, WorkflowDraftResponse
from app.workflow.service import WorkflowService

router = APIRouter(prefix="/v1/workflows", tags=["workflows"])


@router.post("/draft", response_model=WorkflowDraftResponse)
async def draft_workflow(
    request: WorkflowDraftRequest,
    settings: SettingsDep,
) -> WorkflowDraftResponse:
    try:
        service = WorkflowService(create_llm_provider(settings))
        return await service.draft(request)
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
