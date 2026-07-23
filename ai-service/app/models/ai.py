from datetime import datetime
from typing import Any

from sqlalchemy import (
    Boolean,
    DateTime,
    Float,
    ForeignKey,
    Index,
    Integer,
    String,
    Text,
    UniqueConstraint,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.models.base import Base, TimestampMixin, UUIDPrimaryKeyMixin


class AIAgent(UUIDPrimaryKeyMixin, TimestampMixin, Base):
    __tablename__ = "ai_agents"

    name: Mapped[str] = mapped_column(String(120), nullable=False)
    description: Mapped[str | None] = mapped_column(Text)
    system_prompt: Mapped[str] = mapped_column(Text, nullable=False)
    provider: Mapped[str] = mapped_column(String(80), nullable=False, default="openai-compatible")
    model: Mapped[str] = mapped_column(String(160), nullable=False)
    temperature: Mapped[float] = mapped_column(Float, nullable=False, default=0.2)
    enabled: Mapped[bool] = mapped_column(Boolean, nullable=False, default=True)
    metadata_: Mapped[dict[str, Any]] = mapped_column(
        "metadata",
        JSONB,
        nullable=False,
        default=dict,
    )


class AIRequest(UUIDPrimaryKeyMixin, TimestampMixin, Base):
    __tablename__ = "ai_requests"
    __table_args__ = (
        UniqueConstraint("idempotency_key", name="uq_ai_requests_idempotency_key"),
        Index("ix_ai_requests_channel_user", "channel_id", "user_id"),
    )

    idempotency_key: Mapped[str | None] = mapped_column(String(160))
    request_type: Mapped[str] = mapped_column(String(80), nullable=False)
    status: Mapped[str] = mapped_column(String(40), nullable=False, default="pending")
    channel_id: Mapped[str | None] = mapped_column(String(64))
    user_id: Mapped[str | None] = mapped_column(String(64))
    payload_hash: Mapped[str] = mapped_column(String(128), nullable=False)
    request_payload: Mapped[dict[str, Any]] = mapped_column(JSONB, nullable=False, default=dict)
    error_message: Mapped[str | None] = mapped_column(Text)

    responses: Mapped[list["AIResponse"]] = relationship(back_populates="request")


class AIResponse(UUIDPrimaryKeyMixin, TimestampMixin, Base):
    __tablename__ = "ai_responses"

    request_id: Mapped[str] = mapped_column(ForeignKey("ai_requests.id"), nullable=False)
    provider: Mapped[str] = mapped_column(String(80), nullable=False)
    model: Mapped[str | None] = mapped_column(String(160))
    content: Mapped[str] = mapped_column(Text, nullable=False)
    prompt_tokens: Mapped[int | None] = mapped_column(Integer)
    completion_tokens: Mapped[int | None] = mapped_column(Integer)
    total_tokens: Mapped[int | None] = mapped_column(Integer)
    raw_response_id: Mapped[str | None] = mapped_column(String(160))

    request: Mapped[AIRequest] = relationship(back_populates="responses")


class AIWorkflow(UUIDPrimaryKeyMixin, TimestampMixin, Base):
    __tablename__ = "ai_workflows"
    __table_args__ = (Index("ix_ai_workflows_channel_user", "channel_id", "user_id"),)

    workflow_type: Mapped[str] = mapped_column(String(80), nullable=False)
    status: Mapped[str] = mapped_column(String(40), nullable=False, default="draft")
    channel_id: Mapped[str | None] = mapped_column(String(64))
    user_id: Mapped[str | None] = mapped_column(String(64))
    title: Mapped[str | None] = mapped_column(String(240))
    preview_payload: Mapped[dict[str, Any]] = mapped_column(JSONB, nullable=False, default=dict)
    approved_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    rejected_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))


class AIAuditLog(UUIDPrimaryKeyMixin, TimestampMixin, Base):
    __tablename__ = "ai_audit_logs"
    __table_args__ = (Index("ix_ai_audit_logs_request_id", "request_id"),)

    request_id: Mapped[str | None] = mapped_column(ForeignKey("ai_requests.id"))
    event_type: Mapped[str] = mapped_column(String(80), nullable=False)
    actor_user_id: Mapped[str | None] = mapped_column(String(64))
    channel_id: Mapped[str | None] = mapped_column(String(64))
    prompt_hash: Mapped[str | None] = mapped_column(String(128))
    context_refs: Mapped[dict[str, Any]] = mapped_column(JSONB, nullable=False, default=dict)
    policy_decision: Mapped[str | None] = mapped_column(String(80))
    metadata_: Mapped[dict[str, Any]] = mapped_column(
        "metadata",
        JSONB,
        nullable=False,
        default=dict,
    )


class AISetting(UUIDPrimaryKeyMixin, TimestampMixin, Base):
    __tablename__ = "ai_settings"
    __table_args__ = (
        UniqueConstraint("scope", "scope_id", "key", name="uq_ai_settings_scope_key"),
    )

    scope: Mapped[str] = mapped_column(String(40), nullable=False)
    scope_id: Mapped[str] = mapped_column(String(80), nullable=False)
    key: Mapped[str] = mapped_column(String(120), nullable=False)
    value: Mapped[dict[str, Any]] = mapped_column(JSONB, nullable=False, default=dict)


class AIMemory(UUIDPrimaryKeyMixin, TimestampMixin, Base):
    __tablename__ = "ai_memory"
    __table_args__ = (Index("ix_ai_memory_channel_user", "channel_id", "user_id"),)

    channel_id: Mapped[str | None] = mapped_column(String(64))
    user_id: Mapped[str | None] = mapped_column(String(64))
    memory_type: Mapped[str] = mapped_column(String(80), nullable=False)
    content: Mapped[str] = mapped_column(Text, nullable=False)
    embedding_provider: Mapped[str | None] = mapped_column(String(80))
    embedding_model: Mapped[str | None] = mapped_column(String(160))
    embedding_ref: Mapped[str | None] = mapped_column(String(240))
    metadata_: Mapped[dict[str, Any]] = mapped_column(
        "metadata",
        JSONB,
        nullable=False,
        default=dict,
    )
