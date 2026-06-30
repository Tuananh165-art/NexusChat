# Clean Code And Design Patterns

This document defines the implementation standard for NexusChat. It is intentionally specific to the current Go microservice backend and Next.js frontend.

## Goals

- Make service behavior easy to read from use-case methods.
- Keep infrastructure choices replaceable behind small interfaces.
- Prevent message, cache, and API formats from spreading through unrelated code.
- Keep UI pages maintainable as chat features grow.
- Make production changes reviewable through explicit contracts and tests.

## Backend Layering Standard

Each `pkg/<service>` package should follow these responsibilities:

- `domain.go`: domain constants, entity structs, permissions, pure helpers, and invariant checks.
- `service.go`: use cases, orchestration, authorization decisions, event selection, and error context.
- `repo.go` or focused repository files: persistence, cache, broker, gRPC client calls, pagination, and query details.
- `http_api.go` / `grpc_rpc.go`: request decoding, service invocation, presenter conversion, and transport status mapping.
- `router.go`: route registration and middleware composition only.
- `presenter.go`: output DTOs and serialization-friendly shapes.
- `error.go`: sentinel errors and mapping-friendly error vocabulary.

When a file starts mixing multiple responsibilities, split by responsibility before adding more branches.

## Required Patterns

### Dependency Injection

Use constructor injection and Wire-generated composition for concrete dependencies. Service structs should depend on interfaces that express the use case needs.

Do:

```go
type MessageServiceImpl struct {
	msgRepo MessageRepoCache
	userRepo UserRepoCache
	sf common.IDGenerator
}
```

Avoid:

```go
var redisClient = redis.NewClient(...)
```

### Repository Pattern

Repositories own data access and external clients. Services should not know Redis keys, Cassandra CQL, Kafka topics, S3 object layout, or gRPC method descriptors.

Repository methods should be named after domain intent, not storage operations, when possible:

- Prefer `GetUserIDBySession`.
- Avoid leaking `GET rc:session:<sid>` into services.

### Factory/Builder Helpers

Use small factory helpers for repeated domain object creation, especially messages and events. This prevents fields such as `Event`, `Payload`, `Time`, and `DeletedForAll` from drifting across methods.

Factory helpers belong near the service or domain that owns the format. Keep them unexported unless another package truly needs them.

### Strategy By Interface

When behavior has multiple infrastructure implementations, model it as an interface. Examples: ID generation, object signing, publishing, session cache, and inter-service clients.

Do not introduce a strategy abstraction for one-off `if` statements.

### Presenter/DTO Pattern

Keep API response shaping out of repositories. Convert domain entities to presenters at transport boundaries or via explicit presenter methods.

### Middleware Chain

HTTP middleware should be composable and side-effect focused: auth context, body limits, CORS, logging, metrics, and tracing. Business decisions should remain in services.

## Error Handling

- Use sentinel errors for domain decisions such as not found, unauthorized, already deleted, or limit exceeded.
- Wrap infrastructure errors with operation context using `%w`.
- Include identifiers in service-level errors when they help debugging.
- Do not log and return the same error in the same layer unless the log adds request-scoped fields unavailable to callers.
- Do not swallow cleanup errors when the cleanup is part of the operation's correctness.

## Context Rules

- Pass `ctx` from transport to every repository/client call.
- Avoid `context.Background()` in request use cases.
- Background contexts are acceptable only in process lifecycle code or explicitly detached jobs.

## Naming Rules

- Use service vocabulary consistently: `channelID`, `userID`, `messageID`, `subscriber`, `accessToken`, `pageState`.
- Prefer positive boolean names: `matched`, `exists`, `hasMoreMessages`.
- Avoid abbreviations unless already established by protocol or library names.
- Fix spelling in touched code when it is local and safe.

## Frontend Structure Standard

Large pages should be decomposed by feature state:

- `hooks/useChatSession.ts`: authentication bootstrap, websocket connection, visibility, reconnection.
- `hooks/useChatMessages.ts`: message merge, optimistic send, seen/delivered/edit/delete/reaction/pin reducers.
- `hooks/useChatUploads.ts`: paste, drag/drop, chunked upload, upload progress.
- `components/chat/*`: message list, composer, header, search overlay, gallery, notification menu, profile modal.
- `lib/*`: API clients, constants, offline persistence, parsing helpers.

UI components should receive data and callbacks. They should not fetch unrelated resources directly when a page-level hook already owns the workflow.

## Frontend State Rules

- Keep websocket event parsing centralized.
- Keep event payload parsers next to constants so format changes are single-point edits.
- Use stable keys and IDs for optimistic UI updates.
- Keep localStorage keys centralized.
- Isolate browser-only APIs behind guards for server rendering compatibility.

## Testing Expectations

- Domain helpers: table tests.
- Services: unit tests with fake repositories/clients.
- Repositories: integration tests or documented manual verification when external infrastructure is required.
- HTTP/gRPC handlers: request/response mapping tests for validation and error cases.
- Frontend hooks/helpers: unit tests for event parsing, message reducers, upload decisions, and localStorage behavior.

Minimum evidence for a backend change is `go test ./...`. Minimum evidence for a frontend change is `npm --prefix frontend run build`, plus focused tests when present.

## Refactor Checklist

- The public contract is unchanged or the contract change is documented.
- Domain behavior moved inward, not outward.
- Repeated payload or DTO construction is centralized.
- Context propagation is preserved.
- Errors still wrap original causes.
- Generated files are not hand edited.
- Tests or build checks were run and recorded.
