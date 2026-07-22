# NexusChat

![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/Tuananh165-art/NexusChat?label=Version&sort=semver&style=flat-square)

<a href="#"><img src="docs/icons/go.svg" alt="Go" height="20"/></a> <a href="#"><img src="docs/icons/python.svg" alt="Python" height="20"/></a> <a href="#"><img src="docs/icons/nextjs.svg" alt="Next.js" height="20"/></a> <a href="#"><img src="docs/icons/kafka.svg" alt="Kafka" height="20"/></a> <a href="#"><img src="docs/icons/redis.svg" alt="Redis" height="20"/></a> <a href="#"><img src="docs/icons/cassandra.svg" alt="Cassandra" height="20"/></a> <a href="#"><img src="docs/icons/postgresql.svg" alt="PostgreSQL" height="20"/></a> <a href="#"><img src="docs/icons/docker.svg" alt="Docker" height="20"/></a> <a href="#"><img src="docs/icons/prometheus.svg" alt="Prometheus" height="20"/></a> <a href="#"><img src="docs/icons/jaeger.svg" alt="Jaeger" height="20"/></a>

NexusChat is a real-time chat platform built on a microservices architecture.

## Architecture overview

```mermaid
flowchart TB
  browser[Browser / Next.js client]
  ingress[HTTP ingress<br/>Traefik locally / ingress-nginx in K3s]

  subgraph edge[Edge and web tier]
    web[web<br/>Go server web<br/>serves frontend/out]
  end

  subgraph go[Go backend: one nexuschat-api image, multiple commands]
    user[user<br/>HTTP + gRPC<br/>local users, Google OAuth, profile/session lookup]
    match[match<br/>WebSocket /api/match<br/>random matching orchestration]
    chat[chat<br/>HTTP + gRPC + WebSocket<br/>channels, messages, roles, search, AI proxy]
    forwarder[forwarder<br/>gRPC only<br/>active subscriber/session routing]
    uploader[uploader<br/>HTTP /api/uploader<br/>proxy, presigned, chunked uploads]
  end

  subgraph ai[Python AI service]
    aisvc[ai-service<br/>FastAPI on 8090<br/>rewrite, streaming rewrite, agents, MCP preview]
  end

  subgraph state[Stateful dependencies outside the app Helm chart]
    redis[(Redis Cluster<br/>online state, cache, matching state)]
    cassandra[(Cassandra<br/>durable chat data)]
    kafka[(Kafka<br/>message fanout events)]
    minio[(MinIO / S3<br/>uploaded object bytes)]
    postgres[(PostgreSQL<br/>AI state, agents, workflow/audit models)]
  end

  subgraph external[External systems]
    google[Google OAuth]
    provider[OpenAI-compatible AI provider]
  end

  subgraph obs[Observability]
    prom[Prometheus metrics]
    jaeger[OTLP tracing / Jaeger]
  end

  browser -->|/, /chat, /_next/*| ingress --> web
  browser -->|/api/user| ingress --> user
  browser -->|/api/match WebSocket| ingress --> match
  browser -->|/api/chat HTTP + WebSocket| ingress --> chat
  browser -->|/api/uploader| ingress --> uploader
  browser -->|/api/ai via rewrite/strip-prefix| ingress --> aisvc

  match -->|create/join channel| chat
  match -->|profile/session lookup| user
  chat -->|subscriber routing| forwarder
  chat -->|AI rewrite proxy| aisvc
  uploader -->|channel forward-auth / fallback auth| chat

  user --> redis
  match --> redis
  chat --> redis
  chat --> cassandra
  chat --> kafka
  forwarder --> kafka
  uploader --> minio
  aisvc --> postgres
  aisvc --> redis

  user --> google
  aisvc --> provider

  web -. metrics/traces .-> prom
  user -. metrics/traces .-> prom
  match -. metrics/traces .-> prom
  chat -. metrics/traces .-> prom
  forwarder -. metrics/traces .-> prom
  uploader -. metrics/traces .-> prom
  aisvc -. metrics .-> prom
  web -. traces .-> jaeger
  chat -. traces .-> jaeger
  aisvc -. traces .-> jaeger
```

### Runtime sequence: sign in, match, chat, upload, and AI rewrite

```mermaid
sequenceDiagram
  autonumber
  actor U as User browser
  participant W as web
  participant US as user service
  participant M as match service
  participant C as chat service
  participant F as forwarder service
  participant UP as uploader service
  participant AI as ai-service
  participant R as Redis Cluster
  participant K as Kafka
  participant DB as Cassandra
  participant S3 as MinIO/S3
  participant P as AI provider

  U->>W: GET / or /chat
  W-->>U: Static Next.js page and assets
  U->>US: Create local user or start Google OAuth
  US->>R: Store/load user session/profile state
  US-->>U: User profile, cookies, access data

  U->>M: Open WebSocket /api/match
  M->>R: Register waiting user / matching state
  M->>US: Validate session/profile through gRPC
  M->>C: Create or resolve channel membership
  C->>DB: Persist channel and membership data
  M-->>U: Match result with channel data

  U->>C: Open WebSocket /api/chat?uid=...&access_token=...
  C->>R: Track online user/session cache
  U->>C: Send message event
  C->>DB: Persist message, reactions, pins, and channel state
  C->>K: Publish channel fanout event
  K-->>F: Consume fanout event
  F-->>C: Route event to active subscribers
  C-->>U: Broadcast realtime message/action event

  U->>UP: POST /api/uploader/upload/proxy or chunked upload APIs
  UP->>C: Authorize channel access when needed
  UP->>S3: Store object bytes or create presigned URLs
  UP-->>U: File metadata, object key, or presigned URL

  U->>C: POST /api/chat/ai/rewrite
  C->>AI: Forward rewrite request with channel context
  AI->>P: OpenAI-compatible completion request
  P-->>AI: Rewritten text / streamed deltas
  AI-->>C: Rewrite response
  C-->>U: Composer-ready rewritten text
```

### CI/CD and deployment workflow

```mermaid
flowchart LR
  dev[Developer push / pull request] --> trigger{GitHub Actions trigger}
  trigger -->|pull_request| validation[Validation only]
  trigger -->|push main, kafka, or v* tag| release[Validation + image release]
  trigger -->|workflow_dispatch| manual[Manual pipeline run]

  validation --> gotest[Go make test]
  validation --> frontend[Frontend npm ci + typecheck/lint + build]
  validation --> aitest[AI service install + ruff + pytest]
  validation --> helm[Helm lint + default/lab template]
  validation --> security[Gitleaks + dependency review + CodeQL + Trivy FS]

  release --> build[Build Docker images]
  build --> apiimg["nexuschat-api:&lt;tag&gt;"]
  build --> webimg["nexuschat-web:&lt;tag&gt;"]
  build --> aiimg["nexuschat-ai-service:&lt;tag&gt;"]
  build --> proxy["Proxy/lab variants<br/>api:proxy-upload-*<br/>web:proxy-upload-*<br/>web:proxy-upload-v2-*"]

  apiimg --> scan[Image Trivy scan]
  webimg --> scan
  aiimg --> scan
  proxy --> scan
  scan --> sbom[Generate SBOM]
  sbom --> sign[Cosign keyless signing]
  sign --> hub[(Docker Hub<br/>docker.io/tuananh165)]

  hub --> deploy{Branch is main?}
  deploy -->|no| done[Images published only]
  deploy -->|yes| runner[Self-hosted runner<br/>labels: self-hosted, linux, x64, k3s-lab]
  runner --> kubeconfig[Use KUBE_CONFIG_B64<br/>or existing runner kubeconfig]
  kubeconfig --> helmdeploy[helm upgrade --install nexuschat<br/>namespace nexuschat-lab<br/>values.yaml + values-lab-4gb.yaml]
  helmdeploy --> rollout[Wait for web, chat, match, user,<br/>uploader, forwarder, ai-service]
  rollout --> smoke[Print live images and run smoke checks]
```

## Components in the repository

| Area | Path | Role |
| --- | --- | --- |
| Go services | `cmd/`, `pkg/`, `internal/wire/`, `proto/` | CLI `server` with `web`, `chat`, `match`, `user`, `uploader`, and `forwarder` subcommands; HTTP/gRPC/WebSocket; Wire DI |
| Frontend | `frontend/` | Next.js 15 + React 19 + TypeScript chat UI; static export served from `frontend/out` by Go `web` |
| AI service | `ai-service/` | FastAPI service for rewrite, streaming rewrite, agent CRUD, workflow drafts, MCP preview, metrics, and PostgreSQL/Alembic |
| Local runtime | `docker-compose.yaml`, `build/`, `ai-service/Dockerfile` | Traefik, Go images, web image, AI image, Kafka, Redis Cluster, Cassandra, MinIO, Postgres, Prometheus, Jaeger |
| Kubernetes app | `deployments/helm/nexuschat` | Helm chart deploy 7 stateless NexusChat services, Service, Ingress, HPA, PDB, ServiceMonitor, NetworkPolicy, ServiceAccount, optional Traefik Middleware |
| Lab profile | `deployments/helm/nexuschat/values-lab-4gb.yaml` | K3s lab 4GB RAM/50GB disk: 1 replica, Recreate rollout, no Consul, no ServiceMonitor, no NetworkPolicy |
| Platform add-ons | `deployments/platform` | Ingress-nginx, monitoring/logging/security/Consul/ArgoCD manifests and lab dashboard ingress references |
| CI/CD | `.github/workflows/devsecops-platform.yml` | Test/build/scan/SBOM/sign images and deploy `main` directly to K3s lab |
| Generated API docs | `docs/user`, `docs/match`, `docs/chat`, `docs/uploader` | Swagger generated by `make doc`; do not hand-edit generated files |

## Runtime services

| Service | Command/Image | Protocol | Main responsibility |
| --- | --- | --- | --- |
| `web` | `server web`, `nexuschat-web` | HTTP | Serve static Next.js pages `/` and `/chat`, `_next` assets, web metrics |
| `user` | `server user`, `nexuschat-api` | HTTP + gRPC | Local user create/read/update, Google OAuth login/callback, profile/session lookup |
| `match` | `server match`, `nexuschat-api` | WebSocket over HTTP | Cookie-authenticated random matching at `/api/match`, creates channel through chat service |
| `chat` | `server chat`, `nexuschat-api` | HTTP + gRPC + WebSocket | Channel membership, message fanout, persistence, roles, notifications, reactions, pins, search/media listing, forward auth, AI rewrite proxy |
| `forwarder` | `server forwarder`, `nexuschat-api` | gRPC | Transient subscriber/session routing for channel fanout |
| `uploader` | `server uploader`, `nexuschat-api` | HTTP | Channel-authorized upload/download, direct proxy upload, presigned S3 URLs, chunked multipart upload |
| `ai-service` | `uvicorn app.main:app` | HTTP/SSE | OpenAI-compatible provider calls, rewrite, streaming rewrite, agents, workflow drafts, MCP tool preview, metrics |

## Product features currently represented in source

- Anonymous/local user creation and Google OAuth sign-in.
- Random user matching through WebSocket `/api/match` and channel creation.
- Realtime chat through WebSocket `/api/chat?uid=...&access_token=...`.
- Message history pagination, channel users, online users, delivery/seen style action events.
- Reply previews, edit/delete-for-all, reactions, pinned messages, keyword search, media gallery/listing.
- Role and notification preference endpoints under `/api/chat/role` and `/api/chat/notification`.
- File upload through `/api/uploader/upload/proxy`, presigned upload/download, and chunked multipart upload endpoints.
- AI rewrite from the chat composer via Go `chat` proxy `/api/chat/ai/rewrite` to Python `ai-service`.
- AI service direct endpoints: `/health`, `/ready`, `/metrics`, `/v1/assistant/rewrite`, `/v1/assistant/rewrite/stream`, `/v1/agents`, `/v1/mcp/tools`, `/v1/mcp/tools/preview`.
- Prometheus metrics on Go services and AI service; OTLP tracing hooks to Jaeger/collector.

## Tech stack and why it is used

| Layer | Technology | How it is used in this project | Problem it solves |
| --- | --- | --- | --- |
| Backend runtime | Go 1.24 | Builds one `server` binary with Cobra subcommands for `web`, `chat`, `match`, `user`, `uploader`, and `forwarder`. | Keeps service binaries small, fast, and easy to deploy while allowing each service to run as an isolated process/container command. |
| HTTP APIs | Gin | Implements public REST-style APIs for user, chat, uploader, and service health/swagger endpoints. | Provides lightweight routing, middleware, request binding, and response handling for latency-sensitive APIs. |
| gRPC + Protobuf | `google.golang.org/grpc`, generated proto packages | Internal service-to-service calls for user/session lookup, channel creation, and forwarder routing. | Avoids brittle direct database coupling between microservices and gives typed internal contracts. |
| Dependency injection | Google Wire | Generates `internal/wire/wire_gen.go` from provider sets. | Makes service construction explicit and testable without runtime reflection containers. |
| Realtime transport | WebSocket through Gin/Melody | `/api/match` coordinates random matching; `/api/chat` carries realtime chat events. | Solves bidirectional low-latency messaging that normal request/response HTTP cannot handle efficiently. |
| Event fanout | Kafka + Sarama/Watermill | Chat publishes message/action events; `forwarder` routes active subscriber sessions. | Decouples message persistence from realtime delivery and enables scalable fanout across service instances. |
| Online/cache/matching state | Redis Cluster | Stores online/session/cache/matching state and short-lived coordination data. | Keeps ephemeral high-write state out of durable databases and supports horizontal service scaling. |
| Durable chat storage | Cassandra | Stores channel/message-oriented chat data. | Handles high-volume append/read chat workloads with partition-friendly storage. |
| Object storage | MinIO/S3-compatible API + AWS SDK v2 | Stores uploaded object bytes; uploader supports proxy upload, presigned upload/download, and multipart flows. | Separates large binary payloads from chat metadata and supports both local MinIO and production S3-compatible storage. |
| Frontend | Next.js 15, React 19, TypeScript, Tailwind CSS 4 | Builds the chat UI as a static export under `frontend/out`; Go `web` serves it in containers. | Gives a typed, modern UI while keeping production serving simple: static files behind the same ingress. |
| AI service | Python 3.12, FastAPI, Pydantic v2, httpx | Implements `/v1/assistant/rewrite`, streaming rewrite, agents, workflow draft foundations, MCP preview, health, readiness, and metrics. | Keeps AI/provider/prompt/tooling iteration independent from the Go chat domain and avoids mixing provider-specific logic into the core chat service. |
| AI persistence | PostgreSQL, SQLAlchemy async, Alembic | Stores AI service state such as agents, AI requests/responses, workflow/audit/settings/memory model foundations. | Gives relational consistency and migrations for AI metadata without affecting chat Cassandra schemas. |
| Observability | Prometheus metrics, OpenTelemetry, Jaeger | Go services expose metrics/tracing hooks; AI service exposes `/metrics`; local Compose includes Prometheus and Jaeger. | Makes service health, latency, and distributed request paths inspectable during development and deployment. |
| Local orchestration | Docker Compose + Traefik | Runs web, Go services, AI service, Kafka, Redis Cluster, Cassandra, MinIO, Postgres, Prometheus, and Jaeger locally. | Reproduces the multi-service runtime on one developer machine with a single command. |
| Kubernetes deployment | Helm, K3s lab, ingress-nginx | Helm chart deploys stateless app services; stateful dependencies are installed separately; lab values tune resources for a small cluster. | Provides repeatable deployment manifests while keeping heavy stateful systems outside the app release. |
| CI/CD and supply chain | GitHub Actions, Docker Hub, Trivy, Gitleaks, CodeQL, Syft/Anchore SBOM, Cosign | Validates code, builds images, scans filesystem/images, creates SBOMs, signs images, and deploys `main` to the K3s lab. | Catches defects/secrets/vulnerabilities before deployment and publishes traceable immutable images. |

## Problems addressed by this solution

NexusChat is designed around common problems in realtime messaging systems:

1. Realtime communication must be low-latency and bidirectional.
   - WebSocket endpoints keep chat and random matching interactive without polling.
   - Kafka and the `forwarder` service decouple message ingestion from active subscriber routing.

2. Chat data, online state, file bytes, and AI metadata have different storage requirements.
   - Cassandra stores durable chat/channel data.
   - Redis Cluster stores short-lived online, cache, session, and matching state.
   - MinIO/S3 stores uploaded objects outside message records.
   - PostgreSQL stores AI service metadata and migration-managed relational state.

3. Microservices should not share each other's storage schema.
   - Service boundaries are enforced through HTTP/gRPC contracts and events.
   - `chat`, `match`, `user`, `uploader`, `forwarder`, and `ai-service` each own their responsibility and communicate through explicit APIs.

4. Browser upload flows need to work in local labs and Kubernetes.
   - The uploader supports proxy upload for small files to avoid client-side DNS/public endpoint issues with MinIO.
   - Presigned and multipart upload paths remain available for larger objects and production-style object storage flows.

5. AI features should evolve independently from the core chat system.
   - Go `chat` exposes a stable proxy endpoint, `/api/chat/ai/rewrite`.
   - Python `ai-service` owns prompts, provider calls, streaming, agent/workflow/MCP preview logic, and AI persistence.

6. Local development and lab deployment should be close to production without requiring a large cluster.
   - Docker Compose runs the full local stack.
   - Helm values separate default/staging-like settings from the constrained `values-lab-4gb.yaml` profile.
   - Optional platform manifests are kept out of the critical small-lab deployment path.

7. Release artifacts need repeatability and supply-chain visibility.
   - GitHub Actions validates Go, frontend, AI service, Helm, secrets, and security scans.
   - Docker images are tagged immutably, scanned, accompanied by SBOMs, and signed with Cosign.

## Prerequisites

Install these before building or running the whole project locally:

| Tool | Required for | Notes |
| --- | --- | --- |
| Docker Engine + Compose v2 | Full local stack and Docker image builds | `docker compose version` should work. |
| Go 1.24 | Go services, `make test`, `make build` | `make build` installs Wire and Swag CLI tools into `$(go env GOPATH)/bin`. |
| Node.js 20+ and npm | Frontend install, typecheck, and static export build | CI currently uses Node 20. |
| Python 3.12+ with `venv`/`pip` | AI service lint/test/run | On Debian/Ubuntu install `python3.12-venv` if `python3 -m venv` fails. |
| Helm 3 | Kubernetes chart validation/deployment | Needed for `helm lint`, `helm template`, and `helm upgrade`. |
| kubectl | Kubernetes deployment/verification | Required only for cluster operations. |

Do not commit real `.env`, kubeconfig, OAuth secrets, JWT secrets, S3 credentials, database passwords, or AI provider keys.

## Environment setup

Copy example environment files and fill local values only:

```bash
cp .env.example .env
cp ai-service/.env.example ai-service/.env
```

Minimum local `.env` values used by `docker-compose.yaml` include:

```bash
JWT_SECRET='local-jwt-secret-change-me'
REDIS_PASSWORD='local-redis-pass'
USER_OAUTH_GOOGLE_CLIENTID='replace-me'
USER_OAUTH_GOOGLE_CLIENTSECRET='replace-me'
```

Minimum `ai-service/.env` values for real provider-backed AI behavior include:

```bash
AI_ENDPOINT='https://your-openai-compatible-endpoint/v1'
AI_API_KEY='replace-me'
AI_MODEL='replace-me'
DATABASE_URL='postgresql+asyncpg://nexuschat_ai:nexuschat_ai@ai-postgres:5432/nexuschat_ai'
REDIS_URL='redis://redis-node-0:6379/0'
CHAT_SERVICE_BASE_URL='http://random-chat/api/chat'
```

Placeholders are enough to boot some services, but OAuth and AI provider-backed features require real values.

## Build, run, test, and deploy

### 1. Run the full local stack with Docker Compose

This is the easiest way to run the complete system with all dependencies:

```bash
docker compose up --build -d
docker compose ps
```

Common local endpoints:

| URL | Service |
| --- | --- |
| `http://localhost` | Web app served through Traefik |
| `http://localhost:8080` | Traefik dashboard |
| `http://localhost:9001` | MinIO console |
| `http://localhost:9090` | Prometheus |
| `http://localhost:16686` | Jaeger |
| `http://localhost/api/user/swagger/index.html` | User Swagger |
| `http://localhost/api/match/swagger/index.html` | Match Swagger |
| `http://localhost/api/chat/swagger/index.html` | Chat Swagger |
| `http://localhost/api/uploader/swagger/index.html` | Uploader Swagger |
| `http://localhost/api/ai/health` | AI service health through Traefik strip-prefix routing |

Useful local checks:

```bash
docker compose logs -f random-chat
docker compose logs -f ai-service
curl -I http://localhost
curl -i http://localhost/api/ai/health
```

Stop or reset the local stack:

```bash
docker compose down
# Remove volumes only when you intentionally want to delete local Kafka/Redis/Cassandra/MinIO/Postgres data:
docker compose down -v
```

### 2. Build and test Go backend services

Run the canonical Go test target:

```bash
make test
```

Run direct Go tests without the Makefile coverage flags:

```bash
go test ./...
```

Build the production Go binary:

```bash
make build
./server --help
```

`make build` runs dependency generation first:

```bash
make dep   # installs/runs Wire for internal/wire
make doc   # regenerates Swagger docs for chat, match, uploader, user
```

Run individual Go services locally after building the binary. They still need their runtime dependencies and environment variables:

```bash
./server web
./server user
./server match
./server chat
./server uploader
./server forwarder
```

Build Docker images manually:

```bash
make docker-api
make docker-web
# or both:
make docker
```

The Makefile tags those local images as:

```text
tuananh165/nexuschat-api:kafka
tuananh165/nexuschat-web:kafka
```

### 3. Build, run, and test the frontend

Install dependencies:

```bash
npm --prefix frontend ci
```

Run the development server:

```bash
npm --prefix frontend run dev
```

Typecheck/lint and build static output:

```bash
npm --prefix frontend run lint
npm --prefix frontend run build
```

The Next.js build exports static files to `frontend/out`. The Go `web` service and `build/Dockerfile.web` serve that static output in production-style containers.

### 4. Build, run, and test the AI service

Create an isolated Python environment:

```bash
cd ai-service
python3 -m venv .venv
. .venv/bin/activate
python -m pip install -e ".[dev]"
```

Run lint and tests:

```bash
python -m ruff check .
python -m pytest
# or via the AI service Makefile:
make lint
make test
```

Run database migrations and start the service:

```bash
python -m alembic upgrade head
python -m uvicorn app.main:app --host 0.0.0.0 --port 8090 --reload
# or:
make run
```

Direct AI service smoke tests:

```bash
curl -i http://localhost:8090/health
curl -X POST http://localhost:8090/v1/assistant/rewrite \
  -H 'Content-Type: application/json' \
  -d '{"text":"hello, please rewrite this message","tone":"professional","locale":"English"}'
```

Through Docker Compose and Traefik:

```bash
curl -i http://localhost/api/ai/health
curl -X POST http://localhost/api/ai/v1/assistant/rewrite \
  -H 'Content-Type: application/json' \
  -d '{"text":"hello, please rewrite this message","tone":"professional","locale":"English"}'
```

### 5. Validate Kubernetes manifests locally

The application Helm chart is under `deployments/helm/nexuschat`.

```bash
helm lint deployments/helm/nexuschat
helm template nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-staging > /tmp/nexuschat.yaml
helm template nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-4gb.yaml > /tmp/nexuschat-lab.yaml
```

The chart deploys only stateless NexusChat application services. It does not install Kafka, Redis, Cassandra, MinIO/S3, PostgreSQL, ingress-nginx, cert-manager, Prometheus/Grafana, ELK, Consul, or ArgoCD. Install those separately or use managed services, then point Helm values to the correct endpoints.

### 6. Deploy to a K3s lab manually with existing images

Target lab defaults documented in this repository:

| Setting | Value |
| --- | --- |
| Namespace | `nexuschat-lab` |
| Helm release | `nexuschat` |
| Chart | `deployments/helm/nexuschat` |
| Lab values | `deployments/helm/nexuschat/values-lab-4gb.yaml` |
| Ingress controller | ingress-nginx |
| Lab IP | `192.168.109.131` |

Create or update the runtime secret before deploying. Use real values for OAuth and AI features:

```bash
kubectl create namespace nexuschat-lab --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret generic nexuschat-runtime \
  --namespace nexuschat-lab \
  --from-literal=CHAT_JWT_SECRET='change-me' \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --from-literal=CASSANDRA_USER='admin' \
  --from-literal=CASSANDRA_PASSWORD="$CASSANDRA_PASSWORD" \
  --from-literal=UPLOADER_S3_ACCESSKEY="$MINIO_ACCESS_KEY" \
  --from-literal=UPLOADER_S3_SECRETKEY="$MINIO_SECRET_KEY" \
  --from-literal=USER_OAUTH_GOOGLE_CLIENTID="$GOOGLE_CLIENT_ID" \
  --from-literal=USER_OAUTH_GOOGLE_CLIENTSECRET="$GOOGLE_CLIENT_SECRET" \
  --from-literal=DATABASE_URL="$DATABASE_URL" \
  --from-literal=AI_ENDPOINT="$AI_ENDPOINT" \
  --from-literal=AI_API_KEY="$AI_API_KEY" \
  --from-literal=AI_MODEL="$AI_MODEL" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Deploy a known image tag:

```bash
export TAG='<existing-git-sha-or-release-tag>'
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-4gb.yaml \
  --set-string imageDefaults.tag="$TAG" \
  --set-string services.web.image.fullname="docker.io/tuananh165/nexuschat-web:proxy-upload-v2-$TAG" \
  --set-string services.uploader.image.fullname="docker.io/tuananh165/nexuschat-api:proxy-upload-$TAG" \
  --wait --timeout 10m
```

If the selected tag does not have the proxy variant images, remove the two `services.*.image.fullname` overrides or set them to image tags that exist.

Verify rollout:

```bash
kubectl -n nexuschat-lab get pods -o wide
kubectl -n nexuschat-lab get svc
kubectl -n nexuschat-lab get ingress
kubectl -n nexuschat-lab get deploy -o jsonpath='{range .items[*]}{.metadata.name}{" => "}{range .spec.template.spec.containers[*]}{.image}{" "}{end}{"\n"}{end}'

for deploy in web chat match user uploader forwarder ai-service; do
  kubectl -n nexuschat-lab rollout status deployment/$deploy --timeout=600s
done

curl -I http://192.168.109.131
curl -i http://192.168.109.131/api/ai/health
```

Rollback:

```bash
helm history nexuschat -n nexuschat-lab
helm rollback nexuschat <REVISION> -n nexuschat-lab
for deploy in web chat match user uploader forwarder ai-service; do
  kubectl -n nexuschat-lab rollout status deployment/$deploy --timeout=600s
done
```

### 7. Deploy through GitHub Actions CI/CD

Primary workflow: `.github/workflows/devsecops-platform.yml`.

Triggers:

- Pull requests: validation only.
- Push to `main` or `kafka`, and `v*` tags: validation, image build, scan, SBOM, and signing.
- Push to `main`: direct K3s lab deployment from a self-hosted runner.
- Manual: `workflow_dispatch`.

Required GitHub Actions secrets:

| Secret | Required | Purpose |
| --- | --- | --- |
| `DOCKER_USERNAME` | yes | Docker Hub username. |
| `DOCKER_PASSWORD` | yes | Docker Hub password or access token. |
| `KUBE_CONFIG_B64` | conditionally | Base64 kubeconfig for lab deployment; optional only when the self-hosted runner already has kubeconfig. |

The deploy job requires a runner with labels:

```text
self-hosted, linux, x64, k3s-lab
```

Create the kubeconfig secret when needed:

```bash
base64 -w0 ~/.kube/config
```

CI deploys `main` with the equivalent command:

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-4gb.yaml \
  --set-string imageDefaults.tag="${GITHUB_SHA}" \
  --set-string services.web.image.fullname="docker.io/tuananh165/nexuschat-web:proxy-upload-v2-${GITHUB_SHA}" \
  --set-string services.uploader.image.fullname="docker.io/tuananh165/nexuschat-api:proxy-upload-${GITHUB_SHA}" \
  --timeout 10m \
  --wait
```

Watch CI/CD:

```bash
gh run list --workflow "DevSecOps Platform Pipeline" --limit 5
gh run watch
```

### 8. Recommended verification matrix

Run the checks that match the area you changed:

| Area changed | Minimum checks |
| --- | --- |
| Go backend | `make test`, `make build` |
| Frontend | `npm --prefix frontend ci`, `npm --prefix frontend run lint`, `npm --prefix frontend run build` |
| AI service | `python -m ruff check .`, `python -m pytest` inside `ai-service` |
| Helm/deployment config | `helm lint deployments/helm/nexuschat` and both `helm template` commands above |
| Docker images | `make docker-api`, `make docker-web`, or `docker compose build` |
| Documentation only | Markdown link/path sanity, stale reference search, and `git diff --check` |

Generated Swagger docs for Go services:

```bash
make doc
```

## Documentation index

- `docs/README.md`: engineering docs index.
- `docs/architecture.md`: source-accurate architecture and service boundaries.
- `docs/deploy-k8s-guide.md`: K3s lab deployment and CI/CD operations runbook.
- `docs/dockerhub-direct-k8s-rollout.md`: CI/CD quick guide for Docker Hub + direct K8s lab rollout.
- `docs/devsecops-platform-plan.md`: current DevSecOps architecture.
- `docs/devsecops-implementation-runbook.md`: implementation and operations runbook.
- `ai-service/README.md`: AI service setup and API.
