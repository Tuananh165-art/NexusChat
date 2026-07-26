# NexusChat AI Service Plan and Current State

## 1. Current status

`ai-service` has been implemented as an independent Python FastAPI microservice under `ai-service/`. It is deployed by Docker Compose and by the Helm chart as service key `services.ai-service`.

Current implemented capabilities visible in source:

- FastAPI app factory and routers.
- Health/readiness endpoints: `/health`, `/ready`.
- Prometheus metrics endpoint: `/metrics`.
- Assistant rewrite endpoint: `/v1/assistant/rewrite`.
- SSE rewrite endpoint: `/v1/assistant/rewrite/stream`.
- OpenAI-compatible provider adapter configured by `AI_ENDPOINT`, `AI_API_KEY`, `AI_MODEL`.
- Agent create/list/get endpoints under `/v1/agents`.
- MCP tools list and preview endpoints under `/v1/mcp`.
- SQLAlchemy/Alembic baseline for AI state tables.
- Tests under `ai-service/tests`.
- Go `chat` service integration through `/api/chat/ai/rewrite` and `AI_BASEURL`.

## 2. Architecture rule

AI-specific logic stays in Python `ai-service`:

- provider selection and HTTP calls,
- prompt construction,
- context building,
- agent/workflow/MCP policy,
- AI persistence/audit/memory models,
- streaming response format.

Go `chat` remains source of truth for chat business logic and only calls AI service through stable HTTP contracts.

## 3. Runtime integration

Local Compose:

- Traefik exposes `ai-service` at `/api/ai` and strips the prefix.
- Go `chat` uses `AI_BASEURL=http://ai-service:8090`.
- PostgreSQL service `ai-postgres` backs AI persistence.

Kubernetes/Helm:

- Service name: `ai-service`.
- Container port: `8090`.
- Ingress path: `/api/ai(/|$)(.*)` with nginx rewrite target `/$2`.
- Lab command runs Alembic migration before Uvicorn.
- Runtime config comes from Helm env plus secret `nexuschat-runtime`.

## 4. Source layers

| Layer | Path | Responsibility |
| --- | --- | --- |
| API | `app/api` | FastAPI app/routers/dependencies/status mapping |
| Application | `app/application` | Assistant use cases and orchestration |
| Domain | `app/domain` | Provider/domain errors and LLM abstractions |
| Providers | `app/providers` | OpenAI-compatible provider adapter |
| Prompts | `app/prompts` | Rewrite/workflow prompt templates |
| Context | `app/context` | Chat context client/builder |
| Agents | `app/agents` | Agent service and schemas integration |
| Workflow | `app/workflow` | Workflow draft generation service |
| MCP | `app/mcp` | Registry and policy for preview tools |
| Persistence | `app/models`, `app/repositories`, `migrations` | SQLAlchemy models, database setup, Alembic migrations |
| Observability | `app/observability`, `app/api/routers/metrics.py` | Logging and Prometheus metrics |

## 5. API contract summary

| Method | Path | Status |
| --- | --- | --- |
| `GET` | `/health` | implemented |
| `GET` | `/ready` | implemented |
| `GET` | `/metrics` | implemented |
| `POST` | `/v1/assistant/rewrite` | implemented |
| `POST` | `/v1/assistant/rewrite/stream` | implemented |
| `POST` | `/v1/agents` | implemented |
| `GET` | `/v1/agents` | implemented |
| `GET` | `/v1/agents/{agent_id}` | implemented |
| `GET` | `/v1/mcp/tools` | implemented |
| `POST` | `/v1/mcp/tools/preview` | implemented |

Planned/future endpoints should be added only with tests and documentation updates.

## 6. Data model direction

The AI service owns separate PostgreSQL state instead of writing into chat Cassandra tables directly. Current model layer includes AI-oriented entities for agents, requests/responses, workflows, audit/settings/memory foundations. This keeps chat retention, channel membership, and message correctness independent from AI feature iteration.

## 7. Operational requirements

Required environment for real provider-backed features:

```text
DATABASE_URL=postgresql+asyncpg://...
REDIS_URL=redis://...
AI_ENDPOINT=https://provider-compatible/v1
AI_API_KEY=...
AI_MODEL=...
AI_REQUEST_TIMEOUT_SECONDS=60
CHAT_SERVICE_BASE_URL=http://chat:8081/api/chat
```

Smoke tests:

```bash
curl -i http://localhost:8090/health
curl -X POST http://localhost:8090/v1/assistant/rewrite \
  -H 'Content-Type: application/json' \
  -d '{"text":"hello, please rewrite this sentence","tone":"professional","locale":"en-US"}'
```

Kubernetes checks:

```bash
kubectl -n nexuschat-lab logs deploy/ai-service --tail=100
kubectl -n nexuschat-lab exec deploy/ai-service -- env | grep -E 'AI_|DATABASE_URL|REDIS_URL'
curl -i http://IP/api/ai/health
```

## 8. Roadmap

1. Stabilize assistant rewrite and streaming contracts.
2. Add explicit workflow router when workflow draft API is finalized.
3. Add idempotency keys for mutating AI requests.
4. Expand agent run lifecycle: run, cancel, retry, stream.
5. Add audited MCP execution approvals beyond preview.
6. Add semantic memory/search when embedding provider and retention policy are agreed.
7. Add dashboards/alerts for provider latency, provider error rate, token/cost counters, DB migration status.

## 9. Coding rules

- Every new router needs request/response schemas and tests.
- Provider-facing logic must depend on interfaces/protocols, not direct route code.
- External side effects must be preview-first and auditable.
- Do not import Go chat storage/schema into Python AI service.
- Do not log prompts or provider keys in plaintext.
