from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI

from app.agents.service import AgentRegistry
from app.api.routers import agents, assistant, health, mcp, metrics
from app.config.settings import Settings, get_settings
from app.mcp.registry import create_default_registry
from app.observability.logging import configure_logging


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    settings = get_settings()
    configure_logging(settings)
    app.state.settings = settings
    yield


def create_app(settings: Settings | None = None) -> FastAPI:
    resolved_settings = settings or get_settings()
    configure_logging(resolved_settings)

    app = FastAPI(
        title="NexusChat AI Service",
        version="0.1.0",
        description="Independent AI microservice for NexusChat.",
        lifespan=lifespan,
    )
    app.state.settings = resolved_settings
    app.state.agent_registry = AgentRegistry()
    app.state.mcp_registry = create_default_registry()
    app.include_router(health.router)
    app.include_router(metrics.router)
    app.include_router(assistant.router)
    app.include_router(agents.router)
    app.include_router(mcp.router)
    return app
