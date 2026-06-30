"""AI service baseline

Revision ID: 20260630_0001
Revises:
Create Date: 2026-06-30
"""

from collections.abc import Sequence

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql

revision: str = "20260630_0001"
down_revision: str | None = None
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "ai_agents",
        sa.Column("name", sa.String(length=120), nullable=False),
        sa.Column("description", sa.Text(), nullable=True),
        sa.Column("system_prompt", sa.Text(), nullable=False),
        sa.Column("provider", sa.String(length=80), nullable=False),
        sa.Column("model", sa.String(length=160), nullable=False),
        sa.Column("temperature", sa.Float(), nullable=False),
        sa.Column("enabled", sa.Boolean(), nullable=False),
        sa.Column("metadata", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("id", sa.UUID(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_table(
        "ai_requests",
        sa.Column("idempotency_key", sa.String(length=160), nullable=True),
        sa.Column("request_type", sa.String(length=80), nullable=False),
        sa.Column("status", sa.String(length=40), nullable=False),
        sa.Column("channel_id", sa.String(length=64), nullable=True),
        sa.Column("user_id", sa.String(length=64), nullable=True),
        sa.Column("payload_hash", sa.String(length=128), nullable=False),
        sa.Column("request_payload", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("error_message", sa.Text(), nullable=True),
        sa.Column("id", sa.UUID(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("idempotency_key", name="uq_ai_requests_idempotency_key"),
    )
    op.create_index("ix_ai_requests_channel_user", "ai_requests", ["channel_id", "user_id"])
    op.create_table(
        "ai_responses",
        sa.Column("request_id", sa.UUID(), nullable=False),
        sa.Column("provider", sa.String(length=80), nullable=False),
        sa.Column("model", sa.String(length=160), nullable=True),
        sa.Column("content", sa.Text(), nullable=False),
        sa.Column("prompt_tokens", sa.Integer(), nullable=True),
        sa.Column("completion_tokens", sa.Integer(), nullable=True),
        sa.Column("total_tokens", sa.Integer(), nullable=True),
        sa.Column("raw_response_id", sa.String(length=160), nullable=True),
        sa.Column("id", sa.UUID(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(["request_id"], ["ai_requests.id"]),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_table(
        "ai_workflows",
        sa.Column("workflow_type", sa.String(length=80), nullable=False),
        sa.Column("status", sa.String(length=40), nullable=False),
        sa.Column("channel_id", sa.String(length=64), nullable=True),
        sa.Column("user_id", sa.String(length=64), nullable=True),
        sa.Column("title", sa.String(length=240), nullable=True),
        sa.Column("preview_payload", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("approved_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("rejected_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("id", sa.UUID(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index("ix_ai_workflows_channel_user", "ai_workflows", ["channel_id", "user_id"])
    op.create_table(
        "ai_audit_logs",
        sa.Column("request_id", sa.UUID(), nullable=True),
        sa.Column("event_type", sa.String(length=80), nullable=False),
        sa.Column("actor_user_id", sa.String(length=64), nullable=True),
        sa.Column("channel_id", sa.String(length=64), nullable=True),
        sa.Column("prompt_hash", sa.String(length=128), nullable=True),
        sa.Column("context_refs", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("policy_decision", sa.String(length=80), nullable=True),
        sa.Column("metadata", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("id", sa.UUID(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(["request_id"], ["ai_requests.id"]),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index("ix_ai_audit_logs_request_id", "ai_audit_logs", ["request_id"])
    op.create_table(
        "ai_settings",
        sa.Column("scope", sa.String(length=40), nullable=False),
        sa.Column("scope_id", sa.String(length=80), nullable=False),
        sa.Column("key", sa.String(length=120), nullable=False),
        sa.Column("value", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("id", sa.UUID(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("scope", "scope_id", "key", name="uq_ai_settings_scope_key"),
    )
    op.create_table(
        "ai_memory",
        sa.Column("channel_id", sa.String(length=64), nullable=True),
        sa.Column("user_id", sa.String(length=64), nullable=True),
        sa.Column("memory_type", sa.String(length=80), nullable=False),
        sa.Column("content", sa.Text(), nullable=False),
        sa.Column("embedding_provider", sa.String(length=80), nullable=True),
        sa.Column("embedding_model", sa.String(length=160), nullable=True),
        sa.Column("embedding_ref", sa.String(length=240), nullable=True),
        sa.Column("metadata", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("id", sa.UUID(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index("ix_ai_memory_channel_user", "ai_memory", ["channel_id", "user_id"])


def downgrade() -> None:
    op.drop_index("ix_ai_memory_channel_user", table_name="ai_memory")
    op.drop_table("ai_memory")
    op.drop_table("ai_settings")
    op.drop_index("ix_ai_audit_logs_request_id", table_name="ai_audit_logs")
    op.drop_table("ai_audit_logs")
    op.drop_index("ix_ai_workflows_channel_user", table_name="ai_workflows")
    op.drop_table("ai_workflows")
    op.drop_table("ai_responses")
    op.drop_index("ix_ai_requests_channel_user", table_name="ai_requests")
    op.drop_table("ai_requests")
    op.drop_table("ai_agents")
