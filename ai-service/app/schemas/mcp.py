from pydantic import BaseModel, Field


class MCPToolResponse(BaseModel):
    name: str
    description: str
    requires_approval: bool = True
    enabled: bool = False
    input_schema: dict[str, object] = Field(default_factory=dict)


class MCPToolPreviewRequest(BaseModel):
    tool_name: str = Field(min_length=1, max_length=120)
    arguments: dict[str, object] = Field(default_factory=dict)


class MCPToolPreviewResponse(BaseModel):
    tool_name: str
    status: str
    requires_approval: bool
    preview: dict[str, object]
