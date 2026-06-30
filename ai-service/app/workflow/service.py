import json
from typing import Any

from app.domain.llm import LLMProvider
from app.prompts.workflow import build_workflow_prompt
from app.schemas.workflow import WorkflowDraftRequest, WorkflowDraftResponse


class WorkflowService:
    def __init__(self, provider: LLMProvider) -> None:
        self._provider = provider

    async def draft(self, request: WorkflowDraftRequest) -> WorkflowDraftResponse:
        response = await self._provider.complete(build_workflow_prompt(request))
        preview = self._parse_preview(response.content)
        return WorkflowDraftResponse(
            workflow_type=request.workflow_type,
            status="preview",
            preview=preview,
            execution_required_approval=True,
        )

    def _parse_preview(self, content: str) -> dict[str, Any]:
        try:
            parsed = json.loads(content)
        except json.JSONDecodeError:
            return {"text": content}
        return parsed if isinstance(parsed, dict) else {"items": parsed}
