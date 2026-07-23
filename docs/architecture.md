# NexusChat Architecture

## Overview

NexusChat is a real-time microservices chat system. The Go backend builds into one `server` binary with multiple subcommands; the main chat/user/match/uploader/forwarder services share the `nexuschat-api` image, while `web`, `ai-service`, `presence`, `notification`, and `call` run as separate images. The Next.js frontend is built as static output and served by the Go `web` service. AI runs as a separate Python FastAPI service.

## Runtime services

| Service | Source | Runtime command | Protocol | Ingress path |
| --- | --- | --- | --- | --- |
| `web` | `pkg/web`, `frontend` | `server web` | HTTP | `/`, `/chat`, `/_next/*` |
| `user` | `pkg/user` | `server user` | HTTP + gRPC | `/api/user` |
| `match` | `pkg/match` | `server match` | WebSocket over HTTP | `/api/match` |
| `chat` | `pkg/chat` | `server chat` | HTTP + gRPC + WebSocket | `/api/chat` |
| `forwarder` | `pkg/forwarder` | `server forwarder` | gRPC | internal only |
| `uploader` | `pkg/uploader` | `server uploader` | HTTP | `/api/uploader` |
| `ai-service` | `ai-service/app` | `uvicorn app.main:app` | HTTP + SSE | `/api/ai` via ingress rewrite/strip-prefix |
| `presence` | `pkg/presence` | `server presence` | HTTP + gRPC + WebSocket | `/api/presence` |
| `notification` | `pkg/notification` | `server notification` | HTTP + gRPC + WebSocket | `/api/notifications` |
| `call` | `pkg/call` | `server call` | HTTP + gRPC + WebSocket | `/api/calls` |

## Service responsibilities

### `web`

- Serves `frontend/out/index.html` at `/` and `frontend/out/chat.html` at `/chat`.
- Serves static Next.js assets under `/_next`.
- Exposes metrics through the shared Go observability middleware.

### `user`

- Local user creation: `POST /api/user`.
- User lookup/profile: `GET /api/user`, `GET /api/user/me`, `PUT /api/user/me`.
- Google OAuth flow: `/api/user/oauth2/google/login` and `/api/user/oauth2/google/callback`.
- gRPC user/session lookup for internal services.

### `match`

- Cookie-authenticated WebSocket endpoint at `/api/match`.
- Maintains random matching workflow in Redis-backed state.
- Calls chat/user services to create or join channels when a match is found.

### `chat`

- Main realtime WebSocket endpoint at `/api/chat?uid=...&access_token=...`.
- Channel membership and users: `/api/chat/users`, `/api/chat/users/online`.
- Message history and channel operations: `/api/chat/channel/messages`, `/api/chat/channel/pins`, `/api/chat/channel/search`, media listing, delete channel.
- Message events include text/file/action plus edit/delete/reaction/pin events represented through service/domain constants.
- Role and notification preference endpoints under `/api/chat/role` and `/api/chat/notification`.
- Forward-auth endpoint `/api/chat/forwardauth` used by uploader ingress/middleware to authorize channel-scoped upload/download access.
- AI rewrite proxy at `/api/chat/ai/rewrite`, delegating provider-specific work to `ai-service`.

### `forwarder`

- gRPC service for subscriber/session-to-channel routing.
- Keeps transient fanout information out of persistent chat storage.

### `uploader`

- Deprecated direct multi-file upload: `POST /api/uploader/upload/files`.
- Presigned upload: `GET /api/uploader/upload/presigned`.
- Proxy upload used by current frontend for small/single-part uploads: `POST /api/uploader/upload/proxy`.
- Chunked upload lifecycle: `/api/uploader/upload/chunk/init`, `/presign`, `/complete`, `/abort`.
- Download support: `/api/uploader/download/presigned`, `/api/uploader/download/file`.
- Uses S3-compatible storage; local and lab use MinIO bucket `myfilebucket`.

### `ai-service`

- System: `GET /health`, `GET /ready`, `GET /metrics`.
- Assistant: `POST /v1/assistant/rewrite`, `POST /v1/assistant/rewrite/stream`.
- Agents: `POST /v1/agents`, `GET /v1/agents`, `GET /v1/agents/{agent_id}`.
- MCP preview: `GET /v1/mcp/tools`, `POST /v1/mcp/tools/preview`.
- Workflow draft support exists in the application layer and tests; expose/extend routers as contracts stabilize.

### Realtime extensions

- `presence` owns device heartbeats and online/last-seen state in Redis, publishing `nexuschat.presence.events.v1`.
- `notification` owns inbox, unread counts, channel preferences and Web Push subscriptions in Cassandra; it consumes chat/call events idempotently.
- `call` owns 1-1 WebRTC signaling and call metadata. Browser media remains P2P; Redis holds active-call locks and Cassandra retains call metadata for 30 days.
- All three services expose health/readiness, typed `.proto` contracts, and gRPC descriptors backed by the shared event envelope in `pkg/realtime`.

## Data ownership

| Owner | Data |
| --- | --- |
| `user` | Profile/session/auth identity records in Redis-backed repositories |
| `match` | Waiting/matching state and match result coordination |
| `chat` | Channel membership, messages, reactions, pins, roles, preferences, search/media query state; Cassandra for durable chat data, Redis for online/cache state, Kafka for fanout |
| `forwarder` | Transient subscriber routing |
| `uploader` | Object names, upload authorization, presigned/proxy upload behavior; object bytes live in S3/MinIO |
| `ai-service` | AI requests/responses, agents, workflow/audit/settings/memory models in PostgreSQL/Alembic; Redis for AI cache/coordination where configured |
| `presence` | Device heartbeats, online state and last-seen in Redis |
| `notification` | Notifications, preferences, push subscriptions and processed-event deduplication in Cassandra |
| `call` | Active call locks in Redis; call metadata in Cassandra with 30-day TTL; no media/SDP persistence |

Cross-service reads/writes must go through HTTP/gRPC contracts or events. Do not couple one service directly to another service's storage schema.

## Infrastructure dependencies

| Dependency | Local Compose | Kubernetes/Lab expectation |
| --- | --- | --- |
| Kafka | `confluentinc/cp-kafka:7.6.0` | `kafka.kafka.svc.cluster.local:9092` |
| Redis | 6-node `bitnamilegacy/redis-cluster:8.2.1` | Redis Cluster endpoints in `values-lab-k3s.yaml`; standalone Redis is not compatible with current Go cluster client |
| Cassandra | `cassandra:4.0` + `cassandra/init.cql` | `cassandra.cassandra.svc.cluster.local:9042`, schema applied separately |
| MinIO/S3 | `minio/minio`, bucket `myfilebucket` | MinIO or S3-compatible API, bucket created before uploader use |
| PostgreSQL | `postgres:16-alpine` for AI | `DATABASE_URL` injected into `ai-service` |
| Ingress | Traefik in Compose | ingress-nginx in current lab/prod Helm path; Traefik Middleware only optional for specific uploader forward-auth profiles |
| Observability | Prometheus + Jaeger | Optional kube-prometheus-stack/Grafana/Jaeger/ELK manifests under `deployments/platform` |
| WebRTC relay | Coturn in Compose | Optional lab Coturn DaemonSet on `turn.nexuschat.click`; production may use external TURN |

## Request flow examples

### User enters chat

1. Browser loads `/` from `web`.
2. User creates local identity or goes through Google OAuth on `user`.
3. Browser opens `/api/match` WebSocket to find a peer.
4. `match` coordinates with `user` and `chat` to create/resolve channel membership.
5. Browser stores user/channel access data and opens `/api/chat` WebSocket.
6. `chat` persists/fetches messages, publishes fanout through Kafka, and routes active subscribers through `forwarder`.

### File upload in current frontend

1. Browser calls uploader APIs with `Authorization: Bearer <access_token>`.
2. In local Compose, Traefik forward-auth calls `chat` `/api/chat/forwardauth` before uploader.
3. In lab 4GB profile, uploader ingress is nginx and Traefik forward-auth is disabled; uploader service contains auth fallback behavior in source.
4. Small uploads use `/api/uploader/upload/proxy` to avoid browser DNS issues with presigned MinIO URLs.
5. Large uploads use chunk init/presign/complete flow.

### AI rewrite

1. User clicks Sparkles in chat composer.
2. Frontend posts to `/api/chat/ai/rewrite`.
3. Go `chat` validates/authenticates channel context and calls `AI_BASEURL` (`http://ai-service:8090` in chart/compose).
4. `ai-service` builds prompts and calls OpenAI-compatible `AI_ENDPOINT` with `AI_API_KEY` and `AI_MODEL`.
5. Rewritten text returns to composer; sending it is a normal chat message.

### Presence, notification and call flow

1. Browser opens presence and notification WebSockets alongside chat and sends a 15-second heartbeat.
2. Chat publishes a versioned `chat.message.created` event; notification consumes it, applies preferences, persists one inbox item and sends Web Push only when no active browser session exists.
3. Browser creates a call through `/api/calls`; call service validates channel membership, locks both users in Redis, publishes `call.ringing`, and relays WebRTC offer/answer/ICE through its WebSocket.
4. Call state transitions publish versioned events; notification creates incoming/missed-call notifications without storing media or signaling payloads.

## Deployment architecture

- Helm creates one Deployment and one ClusterIP Service per enabled NexusChat service.
- Deployments are named `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, `ai-service`, `presence`, `notification`, and `call`.
- Default app ingress class is `nginx`; default host comes from `global.domain`.
- `ai-service` ingress uses regex path `/api/ai(/|$)(.*)` with nginx rewrite target `/$2`.
- `forwarder` has no ingress path and is internal-only.
- Stateful dependencies are external to the app chart.
- Lab uses namespace `nexuschat-lab`, release `nexuschat`, server IP `IP`, and ingress-nginx.

## Reliability and security boundaries

- Every container runs as non-root UID/GID `10001`, drops all capabilities, disables privilege escalation, and has `readOnlyRootFilesystem: true` in the chart.
- The chart can emit HPA, PDB, ServiceMonitor, and NetworkPolicy resources; lab profile disables HPA/ServiceMonitor/NetworkPolicy to fit small clusters.
- Runtime secrets come from `nexuschat-runtime`, not Git.
- CI runs tests, lint/build, Helm render, secret scan, CodeQL, Trivy, SBOM, image signing before deploy.
