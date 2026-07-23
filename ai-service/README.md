# NexusChat AI Service

`ai-service` is NexusChat's independent Python FastAPI microservice. It owns AI/provider/prompt/context/agent/workflow/MCP preview logic and is not embedded directly into the Go `chat` service. Go `chat` calls AI only over HTTP; for example, composer rewrite goes through `/api/chat/ai/rewrite` and then proxies to `AI_BASEURL`.

## Current stack

- Python >= 3.12
- FastAPI + Uvicorn
- Pydantic v2 + pydantic-settings
- httpx for OpenAI-compatible provider calls
- SQLAlchemy async + Alembic + asyncpg for PostgreSQL state
- redis-py for Redis integration
- prometheus-client metrics
- OpenTelemetry hooks
- pytest, pytest-asyncio, respx, ruff, mypy for development

## Source layout

| Path | Role |
| --- | --- |
| `app/main.py` | Entrypoint |
| `app/api/app.py` | FastAPI app factory/router registration |
| `app/api/routers` | Health, metrics, assistant, agents, MCP routers |
| `app/application` | Assistant use cases |
| `app/providers` | OpenAI-compatible provider abstraction |
| `app/context` | Chat/context client/builder |
| `app/agents` | Agent service |
| `app/workflow` | Workflow draft service |
| `app/mcp` | MCP registry/policy |
| `app/models` | SQLAlchemy models |
| `migrations` | Alembic migrations |
| `tests` | Pytest suite |

## Environment

```text
AI_SERVICE_NAME=nexuschat-ai-service
AI_ENV=local
AI_HOST=0.0.0.0
AI_PORT=8090
AI_ENDPOINT=https://your-openai-compatible-endpoint/v1
AI_API_KEY=your-provider-key
AI_MODEL=your-model
AI_REQUEST_TIMEOUT_SECONDS=60
DATABASE_URL=postgresql+asyncpg://nexuschat_ai:nexuschat_ai@localhost:5432/nexuschat_ai
REDIS_URL=redis://localhost:6379/0
CHAT_SERVICE_BASE_URL=http://localhost/api/chat
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4318
```

Do not commit real `.env` files or provider keys.

## Local development

Linux/macOS:

```bash
cd ai-service
python3 -m venv .venv
. .venv/bin/activate
python -m pip install -e ".[dev]"
python -m alembic upgrade head
python -m uvicorn app.main:app --reload --host 0.0.0.0 --port 8090
```

Windows PowerShell:

```powershell
cd ai-service
py -3.12 -m venv .venv
.\.venv\Scripts\Activate.ps1
python -m pip install -e ".[dev]"
python -m alembic upgrade head
python -m uvicorn app.main:app --reload --host 0.0.0.0 --port 8090
```

Run checks:

```bash
cd ai-service
python -m ruff check .
python -m pytest
```

## API endpoints currently implemented

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/health` | Liveness/health |
| `GET` | `/ready` | Readiness |
| `GET` | `/metrics` | Prometheus metrics |
| `POST` | `/v1/assistant/rewrite` | Rewrite text using configured AI provider |
| `POST` | `/v1/assistant/rewrite/stream` | SSE streaming rewrite |
| `POST` | `/v1/agents` | Create agent |
| `GET` | `/v1/agents` | List agents |
| `GET` | `/v1/agents/{agent_id}` | Get agent |
| `GET` | `/v1/mcp/tools` | List MCP preview tools |
| `POST` | `/v1/mcp/tools/preview` | Preview MCP tool result/policy |

Direct smoke test:

```bash
curl -i http://localhost:8090/health
curl -X POST http://localhost:8090/v1/assistant/rewrite \
  -H 'Content-Type: application/json' \
  -d '{"text":"xin chao toi muon viet lai cau nay","tone":"professional","locale":"Vietnamese"}'
```

Through local Docker Compose/Traefik:

```bash
curl -i http://localhost/api/ai/health
curl -X POST http://localhost/api/ai/v1/assistant/rewrite \
  -H 'Content-Type: application/json' \
  -d '{"text":"xin chao toi muon viet lai cau nay","tone":"professional","locale":"Vietnamese"}'
```

## Kubernetes behavior

Helm chart service key is `services.ai-service`.

- Image: `docker.io/tuananh165/nexuschat-ai-service:<tag>` unless overridden.
- Container port: `8090`.
- Health paths: `/health`, `/ready`.
- Ingress path: `/api/ai(/|$)(.*)` with nginx rewrite target `/$2` in default chart.
- Lab command runs Alembic before Uvicorn:
  `python -m alembic upgrade head && uvicorn app.main:app --host 0.0.0.0 --port 8090`.
- Runtime values come from normal env plus secret `nexuschat-runtime`.

Required Kubernetes secret keys for real AI behavior:

- `DATABASE_URL`
- `AI_ENDPOINT`
- `AI_API_KEY`
- `AI_MODEL`
- optionally `AI_POSTGRES_PASSWORD` if also using it to build DB URL/bootstrap Postgres

## Troubleshooting

```bash
kubectl -n nexuschat-lab logs deploy/ai-service --tail=100
kubectl -n nexuschat-lab exec deploy/ai-service -- env | grep -E 'AI_|DATABASE_URL|REDIS_URL'
kubectl -n nexuschat-lab rollout restart deployment/ai-service
kubectl -n nexuschat-lab rollout status deployment/ai-service --timeout=300s
```

If `/api/chat/ai/rewrite` fails but `/api/ai/health` works, test the AI service directly first. Placeholder `AI_ENDPOINT`, fake `AI_API_KEY`, or invalid `AI_MODEL` will break every provider-backed feature even when Kubernetes routing is healthy.
