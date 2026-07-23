# Clean Code And Design Patterns

This document defines implementation standards for the current NexusChat codebase: Go microservices, Next.js frontend, and Python FastAPI AI service.

## Goals

- Make service behavior readable from use-case methods.
- Keep infrastructure choices replaceable behind small interfaces.
- Prevent message/cache/API formats from spreading through unrelated code.
- Keep frontend chat flows maintainable as realtime features grow.
- Keep AI provider/tool logic isolated in `ai-service`.
- Make deployment changes reviewable through explicit contracts and tests.

## Go backend layering

Each `pkg/<service>` package should keep responsibilities separated:

- `domain.go`: constants, entities, permissions, pure helpers, invariants.
- `service.go`: use cases, orchestration, authorization decisions, event selection, error context.
- `repo.go` or focused repository files: Redis, Cassandra, Kafka, S3, gRPC/HTTP client calls and pagination/query details.
- `http.go` / `http_api.go` / `grpc_rpc.go`: route registration, request decoding, service invocation, presenter conversion, transport status mapping.
- `presenter.go`: response DTOs.
- `error.go`: sentinel errors and mapping-friendly vocabulary.

When a file mixes multiple responsibilities, split by responsibility before adding more branches.

## Required Go patterns

### Dependency injection

Use constructor injection and Wire-generated composition. Services should depend on small interfaces expressing use-case needs.

Avoid global infrastructure clients in service logic.

### Repository pattern

Repositories own data access and external clients. Services should not know Redis keys, Cassandra CQL, Kafka topic implementation details, S3 object layout, or gRPC method descriptors.

### Factory/builder helpers

Use small helpers for repeated message/event/presenter construction so fields such as `Event`, `Payload`, `Time`, `DeletedForAll`, `ParentID`, reactions and pin payloads do not drift across methods.

### Strategy by interface

Introduce interfaces for infrastructure variants such as ID generation, object signing, publishing, session cache, inter-service clients, and AI clients. Do not introduce strategy abstractions for one-off branches.

### Middleware chain

HTTP middleware handles cross-cutting transport concerns: auth context, body limits, CORS, logging, metrics, tracing, rate limits. Business decisions stay in services.

## Error handling

- Use sentinel errors for domain decisions such as not found, unauthorized, already deleted, rate limited, or limit exceeded.
- Wrap infrastructure errors with operation context using `%w`.
- Include identifiers when they materially help debugging.
- Do not log and return the same error at the same layer unless the log adds request-scoped fields not available to callers.
- Do not swallow cleanup errors when cleanup is part of operation correctness.

## Context rules

- Pass `ctx` from transport to every repository/client/provider call.
- Avoid `context.Background()` in request use cases.
- Detached background work must be explicit and tied to process lifecycle or durable queue semantics.

## Naming rules

- Use established vocabulary: `channelID`, `userID`, `messageID`, `subscriber`, `accessToken`, `pageState`, `objectKey`, `uploadID`.
- Prefer positive booleans: `matched`, `exists`, `hasMoreMessages`, `enabled`.
- Avoid abbreviations unless already established by protocol/library names.

## Frontend structure

Current frontend is under `frontend/src`. As chat UI grows, keep heavy page logic decomposed by feature:

- `hooks/useChatSession.ts`: auth bootstrap, WebSocket connection, visibility/reconnect.
- `hooks/useChatMessages.ts`: message merge, optimistic send, seen/delivered/edit/delete/reaction/pin reducers.
- `hooks/useChatUploads.ts`: paste/drag/drop/chunked upload/progress.
- `components/chat/*`: message list, composer, header, search, gallery, profile/modal surfaces.
- `lib/*`: API clients, constants, offline persistence, parsing helpers.

UI components should receive data and callbacks. They should not fetch unrelated resources directly when a page-level hook or `lib/api.ts` owns the workflow.

## Frontend state rules

- Keep WebSocket event parsing centralized.
- Keep event payload parsers next to constants.
- Use stable keys and IDs for optimistic UI updates.
- Keep localStorage keys centralized.
- Guard browser-only APIs for server rendering compatibility.
- Keep AI composer behavior behind API client functions; UI should not know provider details.

## Python AI service rules

`ai-service` uses clean architecture boundaries:

- Routers parse/validate HTTP only.
- Application services orchestrate use cases.
- Provider adapters own external OpenAI-compatible HTTP details.
- Prompt builders live under `app/prompts`.
- Agent/workflow/MCP policies stay in Python, not Go chat.
- SQLAlchemy/Alembic persistence stays behind repository/model boundaries.
- Do not log provider keys, full prompts, or sensitive chat context.
- External tool/MCP side effects must be preview-first and audited.

## API and generated docs

- Swagger files under `docs/user`, `docs/match`, `docs/chat`, `docs/uploader` are generated by `make doc`; do not hand-edit generated files.
- Any change to persisted data, cache keys, Kafka payloads, protobuf fields, Swagger contracts, Helm values, or public routes requires documentation/update notes in the PR.

## Testing expectations

Minimum evidence by touched area:

| Area | Minimum check |
| --- | --- |
| Go backend | `go test ./...` or `make test` |
| Frontend | `npm --prefix frontend run lint` and `npm --prefix frontend run build` |
| AI service | `python -m ruff check .` and `python -m pytest` inside `ai-service` |
| Helm/deploy config | `helm lint deployments/helm/nexuschat` plus default and lab `helm template` renders |
| Docs only | Link/path sanity plus no stale references to removed files/flows |

Run the repo's canonical `make test` / `make build` when a change can affect the full workspace.

## Refactor checklist

- Public contract unchanged or contract change documented.
- Domain behavior moved inward, not outward.
- Repeated payload/DTO construction centralized.
- Context propagation preserved.
- Errors still wrap original causes.
- Generated files not hand-edited.
- Tests/build/render checks run and recorded.
