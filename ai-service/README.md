# NexusChat AI Service

Independent Python AI microservice for NexusChat.

This service owns AI-specific behavior: provider abstraction, prompt building, context building, agent execution, streaming, audit logging, semantic memory, and future MCP integration.

Existing Go chat services remain the source of truth for chat business logic.

## Development

```bash
python -m venv .venv
.venv\Scripts\activate
pip install -e ".[dev]"
pytest
uvicorn app.main:app --reload
```

## Environment

```text
AI_SERVICE_NAME=nexuschat-ai-service
AI_ENV=local
AI_HOST=0.0.0.0
AI_PORT=8090
AI_ENDPOINT=
AI_API_KEY=
AI_MODEL=
AI_REQUEST_TIMEOUT_SECONDS=60
DATABASE_URL=postgresql+asyncpg://nexuschat_ai:nexuschat_ai@localhost:5432/nexuschat_ai
REDIS_URL=redis://localhost:6379/0
```
