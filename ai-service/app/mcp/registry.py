from dataclasses import dataclass, field


@dataclass(frozen=True)
class MCPTool:
    name: str
    description: str
    input_schema: dict[str, object] = field(default_factory=dict)
    enabled: bool = False
    requires_approval: bool = True


class MCPToolRegistry:
    def __init__(self) -> None:
        self._tools: dict[str, MCPTool] = {}

    def register(self, tool: MCPTool) -> None:
        self._tools[tool.name] = tool

    def list(self) -> list[MCPTool]:
        return list(self._tools.values())

    def get(self, name: str) -> MCPTool | None:
        return self._tools.get(name)


def create_default_registry() -> MCPToolRegistry:
    registry = MCPToolRegistry()
    registry.register(
        MCPTool(
            name="github_issue",
            description="Draft a GitHub issue from an approved workflow preview.",
            enabled=False,
            input_schema={
                "type": "object",
                "properties": {
                    "title": {"type": "string"},
                    "body": {"type": "string"},
                },
                "required": ["title", "body"],
            },
        )
    )
    registry.register(
        MCPTool(
            name="calendar_event",
            description="Draft a calendar event from meeting-note output.",
            enabled=False,
            input_schema={
                "type": "object",
                "properties": {
                    "title": {"type": "string"},
                    "start": {"type": "string"},
                    "end": {"type": "string"},
                },
                "required": ["title", "start"],
            },
        )
    )
    return registry
