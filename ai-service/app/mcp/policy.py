from app.mcp.registry import MCPTool


class MCPPolicy:
    def can_execute_without_approval(self, tool: MCPTool) -> bool:
        return tool.enabled and not tool.requires_approval

    def build_preview(self, tool: MCPTool, arguments: dict[str, object]) -> dict[str, object]:
        return {
            "tool": tool.name,
            "arguments": arguments,
            "will_execute": False,
            "reason": "MCP tools are preview-only until explicit user approval is implemented.",
        }
