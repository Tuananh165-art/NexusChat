from fastapi import APIRouter
from pydantic import BaseModel

from app.api.dependencies import SettingsDep

router = APIRouter(tags=["system"])


class HealthResponse(BaseModel):
    status: str
    service: str
    environment: str


@router.get("/health", response_model=HealthResponse)
async def health(settings: SettingsDep) -> HealthResponse:
    return HealthResponse(
        status="ok",
        service=settings.service_name,
        environment=settings.environment,
    )


@router.get("/ready", response_model=HealthResponse)
async def ready(settings: SettingsDep) -> HealthResponse:
    return HealthResponse(
        status="ready",
        service=settings.service_name,
        environment=settings.environment,
    )
