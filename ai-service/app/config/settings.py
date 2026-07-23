from functools import lru_cache

from pydantic import Field, HttpUrl, PostgresDsn, RedisDsn
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_prefix="AI_",
        extra="ignore",
        case_sensitive=False,
        populate_by_name=True,
    )

    service_name: str = Field(default="nexuschat-ai-service")
    environment: str = Field(default="local", alias="AI_ENV")
    host: str = Field(default="0.0.0.0")
    port: int = Field(default=8090)

    endpoint: HttpUrl | None = Field(default=None)
    api_key: str | None = Field(default=None)
    model: str | None = Field(default=None)
    request_timeout_seconds: float = Field(default=60.0, gt=0)

    database_url: PostgresDsn | None = Field(default=None, validation_alias="DATABASE_URL")
    redis_url: RedisDsn | None = Field(default=None, validation_alias="REDIS_URL")
    chat_service_base_url: HttpUrl | None = Field(
        default=None,
        validation_alias="CHAT_SERVICE_BASE_URL",
    )
    otel_exporter_otlp_endpoint: str | None = Field(
        default=None,
        validation_alias="OTEL_EXPORTER_OTLP_ENDPOINT",
    )


@lru_cache
def get_settings() -> Settings:
    return Settings()
