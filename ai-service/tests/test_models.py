from app.models import Base


def test_ai_service_metadata_contains_core_tables() -> None:
    assert {
        "ai_agents",
        "ai_requests",
        "ai_responses",
        "ai_workflows",
        "ai_audit_logs",
        "ai_settings",
        "ai_memory",
    }.issubset(Base.metadata.tables.keys())


def test_ai_requests_has_idempotency_constraint() -> None:
    table = Base.metadata.tables["ai_requests"]
    constraint_names = {constraint.name for constraint in table.constraints}

    assert "uq_ai_requests_idempotency_key" in constraint_names
