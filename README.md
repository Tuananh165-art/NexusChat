# NexusChat

![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/Tuananh165-art/NexusChat?label=Version&sort=semver&style=flat-square)
![CI](https://github.com/Tuananh165-art/NexusChat/actions/workflows/docker-api-dev.yml/badge.svg)

<a href="#"><img src="docs/icons/go.svg" alt="Go" height="20"/></a> <a href="#"><img src="docs/icons/python.svg" alt="Python" height="20"/></a> <a href="#"><img src="docs/icons/nextjs.svg" alt="Next.js" height="20"/></a> <a href="#"><img src="docs/icons/kafka.svg" alt="Kafka" height="20"/></a> <a href="#"><img src="docs/icons/redis.svg" alt="Redis" height="20"/></a> <a href="#"><img src="docs/icons/cassandra.svg" alt="Cassandra" height="20"/></a> <a href="#"><img src="docs/icons/postgresql.svg" alt="PostgreSQL" height="20"/></a> <a href="#"><img src="docs/icons/traefik.svg" alt="Traefik" height="20"/></a> <a href="#"><img src="docs/icons/docker.svg" alt="Docker" height="20"/></a> <a href="#"><img src="docs/icons/prometheus.svg" alt="Prometheus" height="20"/></a> <a href="#"><img src="docs/icons/jaeger.svg" alt="Jaeger" height="20"/></a>

> Production-oriented real-time chat platform built as a microservices system -- Go backend, Next.js frontend, and an independent Python AI service.

---

## Problem

Traditional chat systems often fail when they need to balance these requirements at the same time:

- Low-latency WebSocket messaging
- Scalable service boundaries
- Reliable file upload and access control
- Search, pin, reaction, typing, and delivery state
- Room-level authorization
- AI features that should not pollute the core chat domain

When AI logic is placed inside the chat service, the service becomes harder to reason about, harder to test, and harder to scale independently.

## Solution

NexusChat keeps the chat platform split into focused services and adds AI as a separate service:

- **Go services** handle chat, identity, matching, forwarding, and uploads
- **Python AI service** handles provider abstraction, prompts, context, streaming, workflows, audit, and future MCP integration
- The chat service only forwards AI requests and never owns AI business rules

This keeps the chat runtime stable while AI capabilities evolve independently.

---

## Architecture

### Runtime Services

| Service | Lang | Protocol | Description |
|---------|------|----------|-------------|
| `web` | TypeScript | HTTP | Next.js frontend server |
| `user` | Go | HTTP + gRPC | OAuth, profile, session lookup |
| `match` | Go | HTTP | Random matching, wait-list, idempotency |
| `chat` | Go | HTTP + gRPC | WebSocket messaging, channels, reactions, pins, search |
| `forwarder` | Go | gRPC | Subscriber routing, session-to-channel mapping |
| `uploader` | Go | HTTP | File upload, presigned S3 access control |
| `ai-service` | Python | HTTP | AI rewrite, summary, translate, workflows, agents, MCP |

### Boundary Rules

- Transport handlers only parse and validate input
- Services own business use cases
- Repositories and clients own storage and network concerns
- Other services are called only through explicit clients or APIs
- AI-specific logic stays in `ai-service`

### Data Ownership

| Service | Owns |
|---------|------|
| `user` | User profile, session records |
| `match` | Wait-list state, match results |
| `chat` | Channel membership, message history, reactions, pinned messages, roles |
| `forwarder` | Transient subscriber/session routing |
| `uploader` | Upload authorization, object naming |
| `ai-service` | AI requests, responses, workflows, audit logs, settings, memory |

---

## Tech Stack

| Layer | Stack |
|-------|-------|
| <img src="docs/icons/go.svg" alt="Go" height="20"/> **Backend** | Go 1.24, gRPC, Protobuf, Wire DI, Cobra CLI |
| <img src="docs/icons/python.svg" alt="Python" height="20"/> **AI Service** | Python 3.12+, FastAPI, Pydantic, SQLAlchemy, Alembic, httpx |
| <img src="docs/icons/nextjs.svg" alt="Next.js" height="20"/> **Frontend** | Next.js 15, React 19, TypeScript |
| <img src="docs/icons/kafka.svg" alt="Kafka" height="20"/> **Messaging** | Apache Kafka 7.6.0 (KRaft mode) |
| <img src="docs/icons/redis.svg" alt="Redis" height="20"/> **Cache** | Redis Cluster 8.2.1 (6 nodes: 3 masters + 3 replicas) |
| <img src="docs/icons/cassandra.svg" alt="Cassandra" height="20"/> **Chat Storage** | Apache Cassandra 4.0 |
| <img src="docs/icons/postgresql.svg" alt="PostgreSQL" height="20"/> **AI Storage** | PostgreSQL 16 (Alpine) |
| <img src="docs/icons/traefik.svg" alt="Traefik" height="20"/> **API Gateway** | Traefik v3.3 |
| <img src="docs/icons/prometheus.svg" alt="Prometheus" height="20"/> **Metrics** | Prometheus v2.45.0 |
| <img src="docs/icons/jaeger.svg" alt="Jaeger" height="20"/> **Tracing** | OpenTelemetry -> Jaeger (OTLP gRPC/HTTP) |
| <img src="docs/icons/docker.svg" alt="Docker" height="20"/> **Containers** | Docker, Docker Compose v2 |

---

## Features

### Chat Platform

- Real-time WebSocket chat
- Channel authentication with JWT
- User matching with idempotency
- Message persistence and pagination
- Reactions, pinning, editing, deleting, and seen state
- Typing indicators and delivery status
- Media uploads and presigned access control
- Distributed rate limiting for file upload
- Browser history persistence and reconnection behavior

### AI Platform

- Provider abstraction for any OpenAI-compatible endpoint
- Context-aware rewrite, summary, translation, and workflow preview
- SSE streaming foundation
- Agent participant foundation
- Workflow draft generation
- Semantic memory and audit storage
- MCP preview-only foundation
- AI UI controls inside the chat composer

---

## AI Service Design

The AI layer is intentionally separate from the chat core.

### Responsibilities inside `ai-service`

- Prompt building
- Context building
- Provider communication
- Streaming (SSE)
- Agent execution
- Workflow drafting
- Audit logging
- Semantic search
- Memory
- MCP registry and preview policy

### Supported Provider Model

`ai-service` is configured through environment variables and works with any OpenAI-compatible endpoint:

- OpenAI
- OpenRouter
- LiteLLM
- Ollama-compatible gateway
- vLLM
- Self-hosted AI gateway

### AI Service API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check |
| `GET` | `/metrics` | Prometheus metrics |
| `POST` | `/v1/assistant/rewrite` | Rewrite text |
| `POST` | `/v1/assistant/rewrite/stream` | Rewrite with SSE streaming |
| `POST` | `/v1/workflows/draft` | Draft workflow |
| `POST` | `/v1/agents` | Create agent |
| `GET` | `/v1/agents` | List agents |
| `GET` | `/v1/mcp/tools` | List MCP tools |
| `POST` | `/v1/mcp/tools/preview` | Preview MCP tool |

---

## Repository Layout

```
NexusChat/
|-- cmd/                    # Cobra entry points for Go services
|-- pkg/
|   |-- chat/               # Chat service logic, HTTP, gRPC, repos, subscribers
|   |-- user/               # User service logic
|   |-- match/              # Matching workflow
|   |-- forwarder/          # Subscriber routing
|   |-- uploader/           # Upload service
|   |-- web/                # Frontend server
|   |-- common/             # Shared middleware, observability, auth, logging
|   |-- infra/              # Redis, Kafka, Cassandra clients
|   |-- config/             # Configuration structs
|   |-- transport/          # Transport layer
|-- internal/wire/          # Dependency injection wiring
|-- frontend/               # Next.js chat client
|-- ai-service/             # Standalone Python AI microservice
|-- proto/                  # Protobuf contracts
|-- build/                  # Dockerfiles
|-- cassandra/              # CQL schema init scripts
|-- prometheus/             # Prometheus config
|-- docs/                   # Architecture notes and API docs
```

---

## Getting Started

### Prerequisites

- Docker Engine and Docker Compose v2
- Google OAuth credentials for `user` service (for login)
- An OpenAI-compatible AI endpoint (for AI features)

### 1. Configure Environment

Root `.env` -- Go stack runtime variables:

```env
REDIS_PASSWORD=
JWT_SECRET=
CASSANDRA_HOSTS=localhost
CASSANDRA_PORT=9042
CASSANDRA_USER=admin
CASSANDRA_PASSWORD=nexuschat165
CASSANDRA_KEYSPACE=NexusChat
USER_OAUTH_GOOGLE_CLIENTID=
USER_OAUTH_GOOGLE_CLIENTSECRET=
```

`ai-service/.env` -- AI-specific local dev settings:

```env
AI_SERVICE_NAME=nexuschat-ai-service
AI_ENV=local
AI_HOST=0.0.0.0
AI_PORT=8090
AI_ENDPOINT=https://your-openai-compatible-endpoint/v1
AI_API_KEY=your_api_key
AI_MODEL=your_model
AI_REQUEST_TIMEOUT_SECONDS=60
AI_POSTGRES_PASSWORD=nexuschat_ai
DATABASE_URL=postgresql+asyncpg://nexuschat_ai:nexuschat_ai@localhost:5432/nexuschat_ai
REDIS_URL=redis://localhost:6379/0
CHAT_SERVICE_BASE_URL=http://localhost/api/chat
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4318
```

### 2. Start the Full Stack

```bash
docker compose up --build -d
```

This starts:

| Component | Services |
|-----------|----------|
| Frontend | `web` |
| Go Services | `user`, `match`, `random-chat`, `forwarder`, `uploader` |
| AI | `ai-service` |
| Cache | `redis-node-0` through `redis-node-5` (cluster) |
| Queue | `kafka` |
| Chat DB | `cassandra` + `cassandra-init` |
| AI DB | `ai-postgres` |
| Storage | `minio` + `createbucket` |
| Gateway | `reverse-proxy` (Traefik) |
| Metrics | `prometheus` |
| Tracing | `jaeger` |

### 3. Verify Services

```bash
docker compose ps
docker compose logs -f ai-service
docker compose logs -f random-chat
```

### 4. Open the App

| URL | Service |
|-----|---------|
| `http://localhost` | Web app |
| `http://localhost:8080` | Traefik dashboard |
| `http://localhost:9001` | MinIO console |
| `http://localhost:9090` | Prometheus |
| `http://localhost:16686` | Jaeger |
| `http://localhost/api/user/swagger/index.html` | User API docs |
| `http://localhost/api/match/swagger/index.html` | Match API docs |
| `http://localhost/api/chat/swagger/index.html` | Chat API docs |
| `http://localhost/api/uploader/swagger/index.html` | Uploader API docs |

---

## How To Test AI In The UI

The AI controls are inside the chat composer.

### Rewrite Flow

1. Open `http://localhost`
2. Sign in and enter a chat room
3. Type a draft message
4. Click the `Sparkles` button next to the composer
5. Choose one of:
   - `Professional`
   - `Friendly`
   - `Shorter`
6. The rewritten text replaces the draft in the input box
7. Press send to publish it as a normal chat message

### Workflow Preview Flow

1. Type a draft message
2. Click the `Sparkles` button
3. Choose one of:
   - `Tasks`
   - `Notes`
   - `Checklist`
4. The AI preview appears above the composer
5. Review it before doing anything else

### Direct Backend Test

```bash
curl -X POST http://localhost/api/chat/ai/rewrite \
  -H "Authorization: Bearer <channel-token>" \
  -H "Content-Type: application/json" \
  -d '{"text":"xin chao toi muon viet lai cau nay","tone":"professional","locale":"Vietnamese"}'
```

### Direct AI Service Test

```bash
curl -X POST http://localhost/api/ai/v1/assistant/rewrite \
  -H "Content-Type: application/json" \
  -d '{"text":"xin chao toi muon viet lai cau nay","tone":"professional","locale":"Vietnamese"}'
```

---

## Development Commands

### Go Backend

```bash
go test ./...
go build ./...
```

### Frontend

```bash
npm --prefix frontend run build
npm --prefix frontend run dev
```

### AI Service

```bash
cd ai-service
python -m pytest
python -m ruff check .
python -m uvicorn app.main:app --reload
```

---

## Operational Notes

- `chat` remains the source of truth for chat persistence and WebSocket behavior
- AI features are additive and do not replace the existing chat flow
- AI requests are routed through the chat service only when the UI uses the chat composer AI action
- AI database is separate from Cassandra to keep AI state isolated
- AI provider keys should be rotated and never committed to version control

---

## Docs

- [Architecture](docs/architecture.md)
- [AI Service Plan](docs/ai-service-plan.md)
- [Engineering Docs Index](docs/README.md)
- [AI service README](ai-service/README.md)

---

## Docker Tagging Rules

| Event | Ref | Docker Tags |
|-------|-----|-------------|
| `pull_request` | `refs/pull/2/merge` | `pr-2` |
| `push` | `refs/heads/master` | `master` |
| `push` | `refs/heads/releases/v1` | `releases-v1` |
| `push tag` | `refs/tags/v1.2.3` | `v1.2.3`, `latest` |
| `push tag` | `refs/tags/v2.0.8-beta.67` | `v2.0.8-beta.67`, `latest` |

---

<p align="center">
  <sub>Built with Go | Python | Next.js | Kafka | Redis | Cassandra | PostgreSQL | MinIO | Traefik</sub>
</p>
