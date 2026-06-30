# NexusChat
![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/Tuananh165-art/NexusChat?label=Version&sort=semver)
![CI status api](https://github.com/Tuananh165-art/NexusChat/actions/workflows/docker-api-dev.yml/badge.svg)

NexusChat is a production-oriented real-time chat platform built as a microservices system. The core backend is written in Go, the web client is built with Next.js, and the AI layer is implemented as an independent Python service.

## Problem

Traditional chat systems often fail when they need to balance these requirements at the same time:

- low-latency websocket messaging
- scalable service boundaries
- reliable file upload and access control
- search, pin, reaction, typing, and delivery state
- room-level authorization
- AI features that should not pollute the core chat domain

When AI logic is placed inside the chat service, the service becomes harder to reason about, harder to test, and harder to scale independently.

## Solution

NexusChat keeps the chat platform split into focused services and adds AI as a separate service:

- Go services handle chat, identity, matching, forwarding, and uploads
- Python AI service handles provider abstraction, prompts, context, streaming, workflows, audit, and future MCP integration
- the chat service only forwards AI requests and never owns AI business rules

This keeps the chat runtime stable while AI capabilities evolve independently.

## Architecture

### Runtime services

- `web`: Next.js frontend
- `user`: account, OAuth identity, profile, and session lookup
- `match`: random matching workflow and channel creation trigger
- `chat`: channel membership, websocket message flow, message persistence, roles, reactions, pins, search, and media listing
- `forwarder`: maps channel/user sessions to chat subscribers
- `uploader`: file upload and presigned access control
- `ai-service`: independent Python AI microservice

### System diagram

<img width="828" alt="NexusChat architecture" src="https://github.com/user-attachments/assets/b1184f30-7167-45ab-9038-69e7c3a60c2a">

### Boundary rules

- transport handlers only parse and validate input
- services own business use cases
- repositories and clients own storage and network concerns
- other services are called only through explicit clients or APIs
- AI-specific logic stays in `ai-service`

### Data ownership

- `user` owns user profile and session records
- `match` owns wait-list state and match results
- `chat` owns channel membership, message history, reactions, pinned messages, and roles
- `forwarder` owns transient subscriber/session routing
- `uploader` owns upload authorization and object naming
- `ai-service` owns AI requests, responses, workflows, audit logs, settings, and memory

## Tech Stack

| Layer | Stack |
|---|---|
| Backend | 🐹 Go 1.24 |
| AI service | 🐍 Python 3.12+, FastAPI, Pydantic, SQLAlchemy, Alembic, httpx |
| Frontend | ⚛️ Next.js 15, React 19, TypeScript |
| Messaging | 📨 Kafka |
| Cache | ⚡ Redis |
| Chat storage | 🪨 Cassandra |
| AI storage | 🐘 PostgreSQL |
| Uploads | 🪣 MinIO / S3-compatible storage |
| API routing | 🌐 Traefik |
| Observability | 📈 Prometheus, 🔭 OpenTelemetry, Jaeger |
| Testing | ✅ Go test, pytest |
| Containerization | 🐳 Docker, Docker Compose |

## Features

### Chat platform

- real-time websocket chat
- channel authentication with JWT
- user matching with idempotency
- message persistence and pagination
- reactions, pinning, editing, deleting, and seen state
- typing indicators and delivery status
- media uploads and presigned access control
- distributed rate limiting for file upload
- browser history persistence and reconnection behavior

### AI platform

- provider abstraction for any OpenAI-compatible endpoint
- context-aware rewrite, summary, translation, and workflow preview
- SSE streaming foundation
- agent participant foundation
- workflow draft generation
- semantic memory and audit storage
- MCP preview-only foundation
- AI UI controls inside the chat composer

## AI Service Design

The AI layer is intentionally separate from the chat core.

### Responsibilities inside `ai-service`

- prompt building
- context building
- provider communication
- streaming
- agent execution
- workflow drafting
- audit logging
- semantic search
- memory
- MCP registry and preview policy

### Supported provider model

`ai-service` is configured through environment variables and works with any OpenAI-compatible endpoint:

- OpenAI
- OpenRouter
- LiteLLM
- Ollama-compatible gateway
- vLLM
- self-hosted AI gateway

### AI service API

- `GET /health`
- `GET /ready`
- `GET /metrics`
- `POST /v1/assistant/rewrite`
- `POST /v1/assistant/rewrite/stream`
- `POST /v1/workflows/draft`
- `POST /v1/agents`
- `GET /v1/agents`
- `GET /v1/mcp/tools`
- `POST /v1/mcp/tools/preview`

## Repository Layout

- `cmd/`: Cobra entry points for Go services
- `pkg/chat/`: chat service logic, HTTP, gRPC, repositories, subscribers
- `pkg/user/`: user service logic
- `pkg/match/`: matching workflow
- `pkg/forwarder/`: subscriber routing
- `pkg/uploader/`: upload service
- `pkg/web/`: frontend server
- `pkg/common/`: shared middleware, observability, auth, logging
- `pkg/infra/`: Redis, Kafka, Cassandra clients
- `internal/wire/`: dependency injection wiring
- `frontend/`: Next.js chat client
- `ai-service/`: standalone Python AI microservice
- `proto/`: protobuf contracts
- `docs/`: architecture notes and generated API docs

## Getting Started

### Prerequisites

- Docker Engine and Docker Compose v2
- Google OAuth credentials for `user` service if you want login to work end to end
- an OpenAI-compatible AI endpoint if you want AI features to produce real output

### 1. Configure environment

Root `.env` should contain the runtime variables for the Go stack, for example:

```env
REDIS_PASSWORD=pass.123
JWT_SECRET=mysecret
USER_OAUTH_GOOGLE_CLIENTID=
USER_OAUTH_GOOGLE_CLIENTSECRET=
```

`ai-service/.env` contains AI-specific local dev settings and is loaded directly by compose:

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

### 2. Start the full stack

```bash
docker compose up --build -d
```

This starts:

- frontend
- `user`, `match`, `chat`, `forwarder`, `uploader`
- `ai-service`
- Redis
- Kafka
- Cassandra
- `ai-postgres`
- MinIO
- Traefik
- Prometheus
- Jaeger

### 3. Verify services

```bash
docker compose ps
docker compose logs -f ai-service
docker compose logs -f random-chat
```

### 4. Open the app

- `http://localhost` for the web app
- `http://localhost:8080` for Traefik dashboard
- `http://localhost:9001` for MinIO dashboard
- `http://localhost:9090` for Prometheus
- `http://localhost:16686` for Jaeger
- `http://localhost/api/user/swagger/index.html`
- `http://localhost/api/match/swagger/index.html`
- `http://localhost/api/chat/swagger/index.html`
- `http://localhost/api/uploader/swagger/index.html`

## How To Test AI In The UI

The AI controls are inside the chat composer.

### Rewrite flow

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

### Workflow preview flow

1. Type a draft message
2. Click the `Sparkles` button
3. Choose one of:
   - `Tasks`
   - `Notes`
   - `Checklist`
4. The AI preview appears above the composer
5. Review it before doing anything else

### Direct backend test

```bash
curl -X POST http://localhost/api/chat/ai/rewrite \
  -H "Authorization: Bearer <channel_access_token>" \
  -H "Content-Type: application/json" \
  -d "{\"text\":\"xin chao toi muon viet lai cau nay\",\"tone\":\"professional\",\"locale\":\"Vietnamese\"}"
```

### Direct AI service test

```bash
curl -X POST http://localhost/api/ai/v1/assistant/rewrite \
  -H "Content-Type: application/json" \
  -d "{\"text\":\"xin chao toi muon viet lai cau nay\",\"tone\":\"professional\",\"locale\":\"Vietnamese\"}"
```

## Development Commands

### Go backend

```bash
go test ./...
go build ./...
```

### Frontend

```bash
npm --prefix frontend run build
npm --prefix frontend run dev
```

### AI service

```bash
cd ai-service
python -m pytest
python -m ruff check .
python -m uvicorn app.main:app --reload
```

## Operational Notes

- `chat` remains the source of truth for chat persistence and websocket behavior
- AI features are additive and do not replace the existing chat flow
- AI requests are routed through the chat service only when the UI uses the chat composer AI action
- AI database is separate from Cassandra to keep AI state isolated
- AI provider keys should be rotated and never committed to version control

## Docs

- [Architecture](docs/architecture.md)
- [AI Service Plan](docs/ai-service-plan.md)
- [Engineering Docs Index](docs/README.md)
- [AI service README](ai-service/README.md)

## Docker Tagging Rules

| Event | Ref | Docker Tags |
|---|---|---|
| `pull_request` | `refs/pull/2/merge` | `pr-2` |
| `push` | `refs/heads/master` | `master` |
| `push` | `refs/heads/releases/v1` | `releases-v1` |
| `push tag` | `refs/tags/v1.2.3` | `v1.2.3`, `latest` |
| `push tag` | `refs/tags/v2.0.8-beta.67` | `v2.0.8-beta.67`, `latest` |
