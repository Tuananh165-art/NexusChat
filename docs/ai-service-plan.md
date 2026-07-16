# NexusChat AI Service Plan

## 1. Codebase Inspection

NexusChat currently runs a Go microservice chat system with a Next.js frontend. The inspected runtime services are `web`, `user`, `match`, `chat`, `forwarder`, and `uploader`.

The current chat service owns websocket handling, channel membership, message persistence, roles, reactions, pins, keyword search, media listing, and message fanout. It persists chat state in Cassandra, uses Redis for online/cache state, and publishes chat messages through Kafka topic `rc.msg.pub`.

The AI service must be added as an independent Python service. Existing Go services must not be rewritten, and chat business logic must remain in the chat service.

## 2. Current Architecture

Runtime boundaries:

- `web`: frontend server and browser-facing Next.js client.
- `user`: account, OAuth identity, profile, and session lookup.
- `match`: random matching workflow and channel creation trigger.
- `chat`: message lifecycle, websocket transport, channel roles, and message queries.
- `forwarder`: subscriber/session routing.
- `uploader`: file upload authorization and S3-compatible presigned access.

Infrastructure:

- Cassandra owns chat message/channel data.
- Redis owns online users, channel user cache, notification preferences, and short-lived state.
- Kafka provides at-least-once message fanout.
- gRPC handles selected internal service calls with retry, timeout, and circuit breaker behavior.
- Traefik exposes HTTP routes.
- Prometheus and OpenTelemetry are already part of the platform.

## 3. AI Service Proposal

Add `ai-service`, a Python 3.12+ FastAPI microservice with clean architecture.

The AI service owns:

- prompt building
- context building
- LLM provider abstraction
- streaming
- agent execution
- workflow generation
- semantic search
- memory
- audit logging
- MCP tool integration
- AI-specific business rules

The chat service only calls AI service contracts or publishes normal chat messages. It must not embed prompts, provider logic, tool routing, memory logic, or AI workflow rules.

## 4. Integration Strategy

Recommended first transport:

- REST for request/response use cases.
- Server-Sent Events for token streaming.
- Kafka later for async agent participant events and long-running workflows.

Comparison:

- REST is easiest to integrate across Go, Python, browser, and Traefik.
- gRPC is strong for internal typed calls, but adds cross-language generation overhead.
- Kafka is already present and should be used for async fanout and event-driven AI jobs, not direct token streaming.
- Redis Streams are useful for lightweight queues but less aligned with the platform's existing durable broker choice.
- RabbitMQ and NATS are not recommended initially because they add another broker class.

Timeout strategy:

- Synchronous assistant calls: 10-30 seconds.
- Stream setup: 3 seconds.
- Stream runtime: 2-5 minutes depending on feature.
- Internal context fetches: 1-3 seconds with bounded message limits.
- Provider calls: configurable request timeout with cancellation support.

Retry strategy:

- Retry idempotent context reads and provider transient failures.
- Do not blindly retry non-idempotent workflow creation.
- Use `Idempotency-Key` for all mutating API calls.

Idempotency:

- Store request key, caller, channel, route, payload hash, and result status.
- Return the original result for duplicate completed requests.
- Return conflict when the same key is reused with a different payload hash.

## 5. Product Brainstorm

### AI Agent Participant

AI behaves as a participant in a channel.

Capabilities:

- mention-triggered replies
- typing/thinking status
- streaming responses
- retry and cancel
- multiple agents per channel
- per-agent system prompt, temperature, provider, model, and permission profile

### Context-Aware Assistant

User-facing assistant actions:

- summarize conversation
- rewrite text
- translate text
- grammar correction
- tone adjustment
- smart reply suggestions
- semantic search
- attachment explanation
- conversation explanation

### AI Workflow Engine

Workflow drafts:

- tasks
- action items
- meeting notes
- checklists
- calendar event drafts
- GitHub issue drafts

Workflows are preview-first. External execution must require explicit approval.

## 6. Technical Design

Layers:

- API: FastAPI routers, dependency injection, request validation, status mapping.
- Application: use cases and orchestration.
- Domain: entities, value objects, permissions, errors, and policies.
- Providers: LLM provider interface and OpenAI-compatible adapter.
- Context: context builders and service clients.
- Agents: agent runtime and execution policy.
- Workflow: workflow draft generation and approval state.
- MCP: internal tool registry and execution policy.
- Repositories: PostgreSQL and Redis adapters.
- Observability: structured logging, metrics, tracing.

Provider rule:

- Business logic depends only on provider interfaces.
- Provider settings come from environment variables.
- OpenAI-compatible endpoints are supported through `AI_ENDPOINT`, `AI_API_KEY`, and `AI_MODEL`.

## 7. Folder Structure

```text
ai-service/
  app/
    api/                 FastAPI routers and HTTP dependencies.
    application/         Use cases and orchestration services.
    domain/              Entities, value objects, policies, and domain errors.
    providers/           LLM provider contracts and adapters.
    agents/              Agent runtime, configs, permissions, and run lifecycle.
    prompts/             Prompt templates and builders.
    context/             Conversation, attachment, and memory context builders.
    workflow/            Workflow draft generation and preview models.
    mcp/                 Internal MCP registry, clients, policies, and audit hooks.
    streaming/           SSE event models, cancellation, and stream state.
    repositories/        Repository interfaces and persistence implementations.
    models/              SQLAlchemy models.
    schemas/             Pydantic API schemas.
    config/              Settings and environment parsing.
    workers/             Background worker entry points.
    observability/       Logging, metrics, and tracing setup.
  migrations/            Alembic migrations.
  tests/                 Pytest suite.
  docs/                  Service-specific docs.
```

## 8. Database Design

Use a separate PostgreSQL database for AI state.

Tables:

- `ai_agents`: agent configuration and status.
- `ai_requests`: request metadata, idempotency key, payload hash, status.
- `ai_responses`: final answer, token usage, model, provider metadata.
- `ai_workflows`: workflow drafts, approval state, and execution state.
- `ai_audit_logs`: prompt hash, context references, policy decisions, and tool calls.
- `ai_settings`: user, channel, and tenant-level AI settings.
- `ai_memory`: durable memory, summaries, and embedding references.

Pros:

- independent evolution from chat data
- clean retention and privacy policy boundaries
- no direct coupling to Cassandra internals
- easier audit and cost accounting

Cons:

- requires API/event-based context synchronization
- adds another datastore to operate

## 9. API Design

Core endpoints:

```text
GET  /health
GET  /ready
GET  /metrics

POST /v1/assistant/rewrite
POST /v1/assistant/summary
POST /v1/assistant/translate
POST /v1/assistant/smart-reply
POST /v1/assistant/semantic-search

POST /v1/agents
GET  /v1/agents
GET  /v1/agents/{agent_id}
PATCH /v1/agents/{agent_id}

POST /v1/agent-runs
GET  /v1/agent-runs/{run_id}
GET  /v1/agent-runs/{run_id}/stream
POST /v1/agent-runs/{run_id}/cancel
POST /v1/agent-runs/{run_id}/retry
```

## 10. Streaming Design

Use SSE for first implementation.

Event types:

- `typing`
- `thinking`
- `token`
- `tool_preview`
- `workflow_preview`
- `final`
- `error`
- `cancelled`

SSE is browser-friendly, works through HTTP infrastructure, and is simpler than bidirectional websocket ownership for AI token streams. Existing chat websocket behavior remains owned by the Go chat service.

## 11. MCP Design

MCP is internal to AI service.

Rules:

- Chat service does not know MCP server names, schemas, credentials, or tools.
- AI service exposes high-level preview APIs only.
- Tool execution is preview-first.
- External side effects require explicit approval.
- Every tool attempt is audited.

Planned MCP components:

- `mcp/registry.py`
- `mcp/policy.py`
- `mcp/client.py`
- `mcp/audit.py`

## 12. Implementation Roadmap

Phase 1: Service foundation

- Create Python project skeleton.
- Add FastAPI app factory.
- Add typed environment settings.
- Add health and readiness endpoints.
- Add smoke tests.

Phase 2: Provider abstraction

- Add LLM provider protocol.
- Add OpenAI-compatible adapter using `httpx`.
- Add request/response domain models.
- Add provider unit tests with mocked HTTP transport.

Phase 3: First assistant capability

- Add rewrite endpoint.
- Add prompt builder.
- Add audit placeholder.
- Add tests for validation and provider orchestration.

Phase 4: Persistence

- Add SQLAlchemy async database setup.
- Add Alembic baseline.
- Add request, response, agent, workflow, audit, settings, and memory models.

Phase 5: Streaming

- Add SSE event model.
- Add streaming provider path.
- Add cancellation registry backed by Redis.

Phase 6: Context builder

- Add chat service client.
- Fetch bounded message context from chat REST APIs.
- Add context redaction and token budgeting.

Phase 7: Agent participant

- Add agent config CRUD.
- Add agent run lifecycle.
- Add mention-trigger request schema.
- Add retry, cancel, typing, and thinking states.

Phase 8: Agent runtime

- Expand agent run orchestration.
- Tighten MCP preview and execution safety.
- Extend audit and semantic memory coverage.

Phase 9: Semantic memory/search

- Add memory table and repository.
- Add embedding provider interface.
- Add semantic search endpoint.

Phase 10: MCP integration

- Add internal MCP registry and policy.
- Add preview-only tool calls.
- Add audited approval flow.

Phase 11: Platform integration

- Add Dockerfile.
- Add compose service.
- Add route through Traefik.
- Add Go client only after Python API contract is stable.

## 13. Step-by-Step Coding Rule

Each step must be completed with:

1. Explanation of scope.
2. Code generation.
3. Tests or syntax validation.
4. Fixes for observed errors.
5. Short status before moving to the next step.

Large modules must not be implemented in one pass.
