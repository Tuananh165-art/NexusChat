# NexusChat

![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/Tuananh165-art/NexusChat?label=Version&sort=semver&style=flat-square)

<a href="#"><img src="docs/icons/go.svg" alt="Go" height="20"/></a> <a href="#"><img src="docs/icons/python.svg" alt="Python" height="20"/></a> <a href="#"><img src="docs/icons/nextjs.svg" alt="Next.js" height="20"/></a> <a href="#"><img src="docs/icons/kafka.svg" alt="Kafka" height="20"/></a> <a href="#"><img src="docs/icons/redis.svg" alt="Redis" height="20"/></a> <a href="#"><img src="docs/icons/cassandra.svg" alt="Cassandra" height="20"/></a> <a href="#"><img src="docs/icons/postgresql.svg" alt="PostgreSQL" height="20"/></a> <a href="#"><img src="docs/icons/docker.svg" alt="Docker" height="20"/></a> <a href="#"><img src="docs/icons/prometheus.svg" alt="Prometheus" height="20"/></a> <a href="#"><img src="docs/icons/jaeger.svg" alt="Jaeger" height="20"/></a>

NexusChat is a real-time chat platform built on a microservices architecture. The current source code comprises a Go backend; a Next.js web app (built as static output and served by a Go binary named `web`); a Python FastAPI `ai-service`; Docker Compose for local development; a Helm chart for Kubernetes; and a GitHub Actions workflow that handles building, scanning, signing, and deploying directly to a K3s lab environment.

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

### Kubernetes deployment topology

```mermaid
flowchart TB
  subgraph cluster[K3s lab cluster]
    nginx[ingress-nginx]

    subgraph ns[nexuschat-lab namespace]
      rel[Helm release: nexuschat]
      webd[Deployment/Service: web]
      chatd[Deployment/Service: chat]
      matchd[Deployment/Service: match]
      userd[Deployment/Service: user]
      uploadd[Deployment/Service: uploader]
      forwarderd[Deployment/Service: forwarder]
      aid[Deployment/Service: ai-service]
      secret[nexuschat-runtime Secret]
    end

    subgraph deps[Separate dependency namespaces]
      redisns[redis: Redis Cluster]
      kafkans[kafka: Kafka]
      cassns[cassandra: Cassandra + schema]
      minions[minio: bucket myfilebucket]
      pgns[postgres: AI PostgreSQL]
    end
  end

  nginx -->|/, /chat, /_next| webd
  nginx -->|/api/user| userd
  nginx -->|/api/match| matchd
  nginx -->|/api/chat| chatd
  nginx -->|/api/uploader| uploadd
  nginx -->|/api/ai with rewrite target /$2| aid

  rel --> webd
  rel --> chatd
  rel --> matchd
  rel --> userd
  rel --> uploadd
  rel --> forwarderd
  rel --> aid
  secret -. envFrom .-> webd
  secret -. envFrom .-> chatd
  secret -. envFrom .-> matchd
  secret -. envFrom .-> userd
  secret -. envFrom .-> uploadd
  secret -. envFrom .-> forwarderd
  secret -. envFrom .-> aid

  chatd --> cassns
  chatd --> redisns
  chatd --> kafkans
  matchd --> redisns
  userd --> redisns
  forwarderd --> kafkans
  uploadd --> minions
  aid --> pgns
  aid --> redisns
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

## Tech stack

| Layer | Current stack |
| --- | --- |
| Backend | Go 1.24, Cobra, Wire, Gin, gRPC, Protobuf, go-redis v9 cluster client, gocql, Sarama/Watermill, AWS SDK S3, Prometheus, OpenTelemetry |
| Frontend | Next.js 15.3, React 19.1, TypeScript 5.8, Tailwind CSS 4, Framer Motion, lucide-react |
| AI | Python 3.12, FastAPI, Uvicorn, Pydantic v2, Pydantic Settings, httpx, SQLAlchemy async, Alembic, asyncpg, redis-py, Tenacity, prometheus-client, OpenTelemetry |
| Local infra | Docker Compose, Traefik v3.3, Kafka 7.6.0, Redis Cluster 8.2.1, Cassandra 4.0, MinIO, PostgreSQL 16, Prometheus, Jaeger |
| Kubernetes | K3s lab, Helm 3, ingress-nginx for lab/prod app ingress, optional Traefik Middleware only when enabled for uploader forward-auth |
| CI/CD/Security | GitHub Actions, Docker Hub, Trivy FS/image scans, Gitleaks, dependency review, CodeQL, Syft/Anchore SBOM, Cosign keyless signing, optional Kyverno |

## Local development

Prerequisites: Docker Engine + Compose v2, Go 1.24, Node.js 20, Python 3.12, and real OAuth/AI provider settings when testing those flows.

Copy env examples and fill real values locally only:

```bash
cp .env.example .env
cp ai-service/.env.example ai-service/.env
```

Start full local stack:

```bash
docker compose up --build -d
docker compose ps
```

Common local URLs:

| URL | Service |
| --- | --- |
| `http://localhost` | Web app |
| `http://localhost:8080` | Traefik dashboard |
| `http://localhost:9001` | MinIO console |
| `http://localhost:9090` | Prometheus |
| `http://localhost:16686` | Jaeger |
| `http://localhost/api/user/swagger/index.html` | User Swagger |
| `http://localhost/api/match/swagger/index.html` | Match Swagger |
| `http://localhost/api/chat/swagger/index.html` | Chat Swagger |
| `http://localhost/api/uploader/swagger/index.html` | Uploader Swagger |

Useful checks:

```bash
docker compose logs -f random-chat
docker compose logs -f ai-service
curl -I http://localhost
curl http://localhost/api/ai/health
```

## Development commands

```bash
make test
go test ./...
make build

npm --prefix frontend ci
npm --prefix frontend run lint
npm --prefix frontend run build

cd ai-service
python -m pip install -e ".[dev]"
python -m ruff check .
python -m pytest
python -m uvicorn app.main:app --reload
```

Generated Swagger docs for Go services:

```bash
make doc
```

## Kubernetes deployment model

The source of truth for the app release is `deployments/helm/nexuschat`.

The chart deploys these services: `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, `ai-service`.

The chart does not deploy Kafka, Redis, Cassandra, MinIO, or PostgreSQL. Install those separately through managed services, vendor charts, or the lab commands in `docs/deploy-k8s-guide.md`, then point Helm values to their service endpoints.

Important chart facts from current source:

- Default namespace/profile is staging-like and uses `global.domain: nexuschat.example.com` plus TLS secret `nexuschat-tls`.
- Default ingress class is `nginx`.
- `values-lab-4gb.yaml` overrides the lab to `global.domain: ""`, disables TLS redirect, disables Consul/ServiceMonitor/NetworkPolicy, and keeps one replica per service.
- Default image registry is `docker.io/tuananh165`.
- `imageDefaults.tag` controls `nexuschat-api`, `nexuschat-web`, and `nexuschat-ai-service` unless a service sets `services.<name>.image.fullname`.
- Lab currently overrides `web` to `docker.io/tuananh165/nexuschat-web:proxy-upload-v2` and `uploader` to `docker.io/tuananh165/nexuschat-api:proxy-upload`; the CI deploy job pins those two to SHA-specific proxy variant tags.
- Deployments and Services are named exactly `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, `ai-service`.

Validate chart locally when Helm is installed:

```bash
helm lint deployments/helm/nexuschat
helm template nexuschat deployments/helm/nexuschat --namespace nexuschat-staging > /tmp/nexuschat.yaml
helm template nexuschat deployments/helm/nexuschat --namespace nexuschat-lab \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-4gb.yaml > /tmp/nexuschat-lab.yaml
```

## CI/CD pipeline: GitHub Actions -> Docker Hub -> K3s lab

Primary workflow: `.github/workflows/devsecops-platform.yml`.

Triggers:

- `pull_request` to any branch: validation only.
- `push` to `main` or `kafka`, and `v*` tags: build/scan/sign images.
- `push` to `main`: deploy to K3s lab through the self-hosted runner labels `[self-hosted, linux, x64, k3s-lab]`.
- `workflow_dispatch`: manual run.

Jobs:

1. `test`: `make test`, `npm ci`, frontend lint/build, AI service install/ruff/pytest.
2. `helm`: `helm lint`, default render, lab 4GB render, upload rendered manifests.
3. `secret-scan`: Gitleaks.
4. `dependency-review`: PR-only dependency review with high severity threshold.
5. `codeql`: Go, JavaScript/TypeScript, Python analysis.
6. `trivy-fs`: repo filesystem scan and SARIF upload.
7. `build-images`: build/push/Trivy-scan/SBOM/sign `nexuschat-api`, `nexuschat-web`, `nexuschat-ai-service` to Docker Hub.
8. `build-proxy-variants`: build additional `nexuschat-api:proxy-upload[-TAG]`, `nexuschat-web:proxy-upload[-TAG]`, `nexuschat-web:proxy-upload-v2[-TAG]`.
9. `deploy-lab-k8s`: only on `main`; writes kubeconfig from `KUBE_CONFIG_B64` when set, verifies cluster, creates `nexuschat-lab`, runs Helm upgrade with `--wait`, waits each deployment, prints actual images.

Required GitHub Actions secrets:

- `DOCKER_USERNAME`
- `DOCKER_PASSWORD` (prefer Docker Hub access token)
- `KUBE_CONFIG_B64` (optional only if the self-hosted runner already has kubeconfig; otherwise required)

The lab deploy command used by CI is effectively:

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

Post-deploy checks:

```bash
kubectl -n nexuschat-lab get pods -o wide
kubectl -n nexuschat-lab get ingress
kubectl -n nexuschat-lab get deploy -o jsonpath='{range .items[*]}{.metadata.name}{" => "}{range .spec.template.spec.containers[*]}{.image}{" "}{end}{"\n"}{end}'
for d in web chat match user uploader forwarder ai-service; do kubectl -n nexuschat-lab rollout status deployment/$d --timeout=600s; done
curl -I http://192.168.109.131
curl -i http://192.168.109.131/api/ai/health
```

## Secrets and runtime config

The Helm chart injects `global.commonEnv` into every container and also `envFrom` secret `nexuschat-runtime` by default.

Create secrets outside Git. At minimum the lab/prod namespace needs values for:

| Key | Used by |
| --- | --- |
| `CHAT_JWT_SECRET` | chat |
| `REDIS_PASSWORD` | Go services |
| `CASSANDRA_USER`, `CASSANDRA_PASSWORD` | Go services |
| `UPLOADER_S3_ACCESSKEY`, `UPLOADER_S3_SECRETKEY` | uploader |
| `USER_OAUTH_GOOGLE_CLIENTID`, `USER_OAUTH_GOOGLE_CLIENTSECRET` | user |
| `DATABASE_URL`, `AI_ENDPOINT`, `AI_API_KEY`, `AI_MODEL` | ai-service |
| `AI_POSTGRES_PASSWORD` | ai-service/Postgres bootstrap references |

Do not commit `.env`, kubeconfig, OAuth secrets, JWT secrets, S3 credentials, database passwords, or AI provider keys.

## Documentation index

- `docs/README.md`: engineering docs index.
- `docs/architecture.md`: source-accurate architecture and service boundaries.
- `docs/deploy-k8s-guide.md`: K3s lab deployment and CI/CD operations runbook.
- `docs/dockerhub-direct-k8s-rollout.md`: CI/CD quick guide for Docker Hub + direct K8s lab rollout.
- `docs/devsecops-platform-plan.md`: current DevSecOps architecture.
- `docs/devsecops-implementation-runbook.md`: implementation and operations runbook.
- `ai-service/README.md`: AI service setup and API.
