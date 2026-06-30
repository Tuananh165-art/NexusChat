from app.domain.llm import LLMMessage, LLMRequest
from app.schemas.workflow import WorkflowDraftRequest


def build_workflow_prompt(request: WorkflowDraftRequest) -> LLMRequest:
    return LLMRequest(
        messages=[
            LLMMessage(
                role="system",
                content=(
                    "You generate workflow drafts from chat content. Return compact JSON only. "
                    "Never claim that any external action has been executed."
                ),
            ),
            LLMMessage(
                role="user",
                content=(
                    f"Create a {request.workflow_type} draft from this source text. "
                    "The result is a preview and must require user approval before execution.\n\n"
                    f"{request.source_text}"
                ),
            ),
        ],
        temperature=0.1,
    )
