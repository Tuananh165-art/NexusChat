from uuid import UUID, uuid4

from app.schemas.agents import AgentCreateRequest, AgentResponse


class AgentRegistry:
    def __init__(self) -> None:
        self._agents: dict[UUID, AgentResponse] = {}

    def create(self, request: AgentCreateRequest) -> AgentResponse:
        agent = AgentResponse(id=uuid4(), **request.model_dump())
        self._agents[agent.id] = agent
        return agent

    def list(self) -> list[AgentResponse]:
        return list(self._agents.values())

    def get(self, agent_id: UUID) -> AgentResponse | None:
        return self._agents.get(agent_id)
