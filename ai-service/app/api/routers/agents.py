from uuid import UUID

from fastapi import APIRouter, HTTPException, Request, status

from app.agents.service import AgentRegistry
from app.schemas.agents import AgentCreateRequest, AgentResponse

router = APIRouter(prefix="/v1/agents", tags=["agents"])


def get_registry(request: Request) -> AgentRegistry:
    return request.app.state.agent_registry


@router.post("", response_model=AgentResponse, status_code=status.HTTP_201_CREATED)
async def create_agent(request_body: AgentCreateRequest, request: Request) -> AgentResponse:
    return get_registry(request).create(request_body)


@router.get("", response_model=list[AgentResponse])
async def list_agents(request: Request) -> list[AgentResponse]:
    return get_registry(request).list()


@router.get("/{agent_id}", response_model=AgentResponse)
async def get_agent(agent_id: UUID, request: Request) -> AgentResponse:
    agent = get_registry(request).get(agent_id)
    if agent is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="agent not found")
    return agent
