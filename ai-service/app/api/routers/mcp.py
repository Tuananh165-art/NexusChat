from fastapi import APIRouter, HTTPException, Request, status

from app.mcp.policy import MCPPolicy
from app.mcp.registry import MCPToolRegistry
from app.schemas.mcp import MCPToolPreviewRequest, MCPToolPreviewResponse, MCPToolResponse

router = APIRouter(prefix="/v1/mcp", tags=["mcp"])


def get_registry(request: Request) -> MCPToolRegistry:
    return request.app.state.mcp_registry


@router.get("/tools", response_model=list[MCPToolResponse])
async def list_tools(request: Request) -> list[MCPToolResponse]:
    return [MCPToolResponse(**tool.__dict__) for tool in get_registry(request).list()]


@router.post("/tools/preview", response_model=MCPToolPreviewResponse)
async def preview_tool(
    request_body: MCPToolPreviewRequest,
    request: Request,
) -> MCPToolPreviewResponse:
    tool = get_registry(request).get(request_body.tool_name)
    if tool is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="MCP tool not found")

    policy = MCPPolicy()
    return MCPToolPreviewResponse(
        tool_name=tool.name,
        status="preview",
        requires_approval=not policy.can_execute_without_approval(tool),
        preview=policy.build_preview(tool, request_body.arguments),
    )
