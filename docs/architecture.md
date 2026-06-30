# NexusChat Architecture

## Runtime Services

NexusChat is organized as stateless services backed by shared infrastructure:

- `web`: frontend server and browser-facing application.
- `user`: account, OAuth identity, profile, and session lookup.
- `match`: random matching workflow and channel creation trigger.
- `chat`: channel membership, websocket message flow, message persistence, roles, reactions, pins, search, and media listing.
- `forwarder`: maps channel/user sessions to chat subscribers.
- `uploader`: file upload and presigned access control.

## Dependency Direction

Keep dependencies moving inward:

1. Transport adapters receive HTTP, websocket, or gRPC requests.
2. Services execute use cases against interfaces.
3. Repositories and clients implement those interfaces with Redis, Cassandra, Kafka, S3-compatible storage, or gRPC.
4. Domain types define invariants and shared vocabulary inside each bounded context.

Services may call other services through explicit client repositories. They should not import another service's storage implementation.

## Data Ownership

- `user` owns user profile and session records.
- `match` owns wait-list state and publishes match results.
- `chat` owns channel membership, message history, reactions, pinned messages, and channel roles.
- `forwarder` owns transient subscriber/session routing.
- `uploader` owns upload authorization, object naming, and presigned access.

Cross-service data should be copied through API/gRPC contracts or events. Do not reach into another service's database tables or Redis keys directly.

## Integration Patterns

- HTTP handlers should translate request/response concerns only.
- gRPC clients should be wrapped behind repository/client interfaces.
- Kafka publishing should stay in repository/client infrastructure, while services decide which domain event to publish.
- Redis and Cassandra key/query details must stay out of service methods.
- Wire should remain the composition root for concrete dependency assembly.

## Reliability Boundaries

- Contexts must flow from the incoming request through all repository and client calls.
- Long-running background operations should derive from service lifecycle context, not `context.Background()` inside request paths.
- Message/event payload formats must be centralized so the sender and receiver evolve together.
- Any change to persisted data, cache keys, Kafka payloads, protobuf fields, or Swagger contracts requires a migration/compatibility note in the PR.
