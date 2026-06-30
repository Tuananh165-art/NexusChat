from typing import Any, Literal

from pydantic import BaseModel, Field

WorkflowType = Literal[
    "tasks",
    "meeting_notes",
    "action_items",
    "checklist",
    "calendar_event",
    "github_issue",
]


class WorkflowDraftRequest(BaseModel):
    workflow_type: WorkflowType
    source_text: str = Field(min_length=1, max_length=20000)
    channel_id: str | None = Field(default=None, max_length=64)
    user_id: str | None = Field(default=None, max_length=64)


class WorkflowDraftResponse(BaseModel):
    workflow_type: WorkflowType
    status: str
    preview: dict[str, Any]
    execution_required_approval: bool = True
