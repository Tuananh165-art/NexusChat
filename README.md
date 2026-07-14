# NexusChat

![GitHub tag (latest SemVer)](https://img.shields.io/github/v/tag/Tuananh165-art/NexusChat?label=Version&sort=semver&style=flat-square)

<a href="#"><img src="docs/icons/go.svg" alt="Go" height="20"/></a> <a href="#"><img src="docs/icons/python.svg" alt="Python" height="20"/></a> <a href="#"><img src="docs/icons/nextjs.svg" alt="Next.js" height="20"/></a> <a href="#"><img src="docs/icons/kafka.svg" alt="Kafka" height="20"/></a> <a href="#"><img src="docs/icons/redis.svg" alt="Redis" height="20"/></a> <a href="#"><img src="docs/icons/cassandra.svg" alt="Cassandra" height="20"/></a> <a href="#"><img src="docs/icons/postgresql.svg" alt="PostgreSQL" height="20"/></a> <a href="#"><img src="docs/icons/docker.svg" alt="Docker" height="20"/></a> <a href="#"><img src="docs/icons/prometheus.svg" alt="Prometheus" height="20"/></a> <a href="#"><img src="docs/icons/jaeger.svg" alt="Jaeger" height="20"/></a>

NexusChat is a production-oriented real-time chat platform built as a microservices system with Go services, a Next.js frontend, and an independent Python AI service. The repository includes local Docker Compose orchestration and a Kubernetes DevSecOps scaffold covering Helm, GitHub Actions, Nginx Ingress, Prometheus/Grafana, ELK, ArgoCD GitOps, Consul service mesh, and security admission controls.

## What This Repository Contains

| Area | Path | Purpose |
| --- | --- | --- |
| Go services | `cmd/`, `pkg/`, `internal/wire/` | Chat, user, match, forwarder, uploader, web server, shared infra |
| Frontend | `frontend/` | Next.js chat client |
| AI service | `ai-service/` | FastAPI AI microservice for assistant, workflow, agents, MCP preview, metrics |
| Docker assets | `docker-compose.yaml`, `build/` | Local full-stack runtime and image builds |
| Kubernetes app chart | `deployments/helm/nexuschat` | Helm chart for NexusChat stateless services |
| GitOps apps | `deployments/gitops/applications` | ArgoCD application definitions |
| Platform config | `deployments/platform` | Nginx, Prometheus/Grafana, ELK/ECK, Consul, Kyverno, ArgoCD values |
| CI/CD | `.github/workflows/devsecops-platform.yml` | Test, scan, build, SBOM, sign, and Helm validation pipeline |
| Docs | `docs/` | Architecture, AI plan, DevSecOps plan, implementation runbook, generated API docs |

## Architecture

### Runtime Services

| Service | Lang | Protocol | Responsibility |
| --- | --- | --- | --- |
| `web` | TypeScript/Go image | HTTP | Browser-facing Next.js app served by the web binary/image |
| `user` | Go | HTTP + gRPC | OAuth, profile, auth cookies, session lookup |
| `match` | Go | HTTP | Random matching workflow and channel creation trigger |
| `chat` | Go | HTTP + gRPC + WebSocket | Channels, messages, roles, reactions, pins, search, AI request forwarding |
| `forwarder` | Go | gRPC | Transient subscriber/session-to-channel routing |
| `uploader` | Go | HTTP | Upload authorization and S3-compatible presigned access |
| `ai-service` | Python | HTTP | AI rewrite, streaming, workflows, agents, MCP preview, audit/memory foundation |

### Data Ownership

| Service | Owns |
| --- | --- |
| `user` | User profile and session records |
| `match` | Wait-list state and match results |
| `chat` | Channel membership, message history, reactions, pins, roles, message search |
| `forwarder` | Transient subscriber and session routing |
| `uploader` | Upload authorization, object naming, presigned URLs |
| `ai-service` | AI requests, provider abstraction, workflow drafts, agent records, audit/memory state |

### Dependency Rules

- Transport handlers parse requests and responses only.
- Service packages own use cases and domain decisions.
- Repositories and clients own Redis, Cassandra, Kafka, S3, gRPC, and HTTP integration details.
- Cross-service access goes through explicit APIs, gRPC clients, or events.
- AI-specific logic stays in `ai-service`; `chat` only forwards AI requests for composer features.

## Tech Stack

| Layer | Stack |
| --- | --- |
| Backend | Go 1.24, Cobra, Wire DI, gRPC, Protobuf, Gin-style HTTP packages |
| Frontend | Next.js 15, React 19, TypeScript |
| AI service | Python 3.12, FastAPI, Pydantic, SQLAlchemy, Alembic, httpx |
| Messaging | Kafka 7.6.0 in local Docker Compose |
| Cache | Redis Cluster 8.2.1 in local Docker Compose |
| Chat storage | Cassandra 4.0 |
| AI storage | PostgreSQL 16 |
| Object storage | MinIO locally, S3-compatible storage in production |
| Local gateway | Traefik v3.3 |
| Production ingress | Nginx Ingress Controller |
| Metrics | Prometheus, kube-prometheus-stack, Grafana |
| Tracing | OpenTelemetry to Jaeger locally; OTLP collector recommended in production |
| Logs | ELK through ECK/Filebeat scaffold |
| GitOps | ArgoCD |
| Service mesh | Consul Connect transparent proxy |
| Security | Gitleaks, dependency review, CodeQL, Trivy, Syft SBOM, Cosign signing, Kyverno |

## Local Development

### Prerequisites

- Docker Engine and Docker Compose v2
- Go 1.24 for backend development
- Node.js 20 for frontend development
- Python 3.12 for AI service development
- Google OAuth credentials for interactive login
- OpenAI-compatible AI endpoint for AI features

### Environment Files

Root `.env`:

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

`ai-service/.env`:

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

Never commit real `.env` files, OAuth secrets, JWT secrets, database passwords, S3 credentials, or AI provider keys.

### Start the Full Local Stack

```bash
docker compose up --build -d
```

Local services:

| URL | Service |
| --- | --- |
| `http://localhost` | Web app |
| `http://localhost:8080` | Traefik dashboard |
| `http://localhost:9001` | MinIO console |
| `http://localhost:9090` | Prometheus |
| `http://localhost:16686` | Jaeger |
| `http://localhost/api/user/swagger/index.html` | User API docs |
| `http://localhost/api/match/swagger/index.html` | Match API docs |
| `http://localhost/api/chat/swagger/index.html` | Chat API docs |
| `http://localhost/api/uploader/swagger/index.html` | Uploader API docs |

Useful checks:

```bash
docker compose ps
docker compose logs -f random-chat
docker compose logs -f ai-service
```

## Development Commands

### Go Backend

```bash
make test
go test ./...
go build ./...
```

### Frontend

```bash
npm --prefix frontend ci
npm --prefix frontend run build
npm --prefix frontend run dev
```

### AI Service

```bash
cd ai-service
python -m pip install -e ".[dev]"
python -m pytest
python -m ruff check .
python -m uvicorn app.main:app --reload
```

### Generated API Docs

Swagger files under `docs/user`, `docs/match`, `docs/chat`, and `docs/uploader` are generated. Use the existing Make target instead of hand editing:

```bash
make doc
```

## DevSecOps Operating Model

NexusChat uses two deployment modes:

- Local development uses Docker Compose and Traefik.
- Production-oriented delivery uses Kubernetes, Helm, GitHub Actions, ArgoCD, Nginx Ingress, Prometheus/Grafana, ELK, Consul mesh, and Kyverno.

Detailed references:

- [DevSecOps Platform Plan](docs/devsecops-platform-plan.md)
- [DevSecOps Implementation Runbook](docs/devsecops-implementation-runbook.md)

### Environment Model

| Environment | Source | Namespace | Delivery |
| --- | --- | --- | --- |
| `dev` | Feature branches and PRs | Preview or ephemeral namespaces | CI validation only |
| `staging` | `main` | `nexuschat-staging` | ArgoCD auto-sync |
| `production` | Signed `v*` tags | `nexuschat-prod` | GitOps promotion with controlled sync |

### Kubernetes and Helm

Application chart:

```text
deployments/helm/nexuschat
```

The chart manages:

- Deployments for `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, and `ai-service`.
- ClusterIP services and Nginx ingress routes.
- Prometheus scrape annotations and `ServiceMonitor` resources.
- Service accounts, pod/container security contexts, resource requests/limits.
- HorizontalPodAutoscalers and PodDisruptionBudgets.
- NetworkPolicy baseline.
- Consul mesh annotations and `ServiceDefaults`.

The chart intentionally does not deploy Kafka, Redis, Cassandra, MinIO, or Postgres. In production, deploy those through managed services or dedicated vendor charts and point NexusChat values to those endpoints.

Important Helm values:

| Value | Purpose |
| --- | --- |
| `global.imageRegistry` | Registry namespace for `api`, `web`, and `ai-service` images |
| `imageDefaults.tag` | Default immutable image tag for the release |
| `global.domain` | Ingress host |
| `global.tlsSecretName` | TLS secret used by Nginx ingress |
| `global.envFromSecrets` | Runtime secret references such as `nexuschat-runtime` |
| `global.commonEnv` | Shared Kafka, Redis, Cassandra, OTLP, and metrics settings |
| `services.*.env` | Service-specific environment variables |
| `serviceMonitor.enabled` | Prometheus Operator scrape integration |
| `networkPolicy.enabled` | Kubernetes network policy baseline |

Local render command when Helm is installed:

```bash
helm lint deployments/helm/nexuschat
helm template nexuschat deployments/helm/nexuschat --namespace nexuschat-staging
```

### GitHub Actions CI/CD

Primary workflow:

```text
.github/workflows/devsecops-platform.yml
```

Pull request gates:

1. Go tests through `make test`.
2. Frontend install, lint, and build.
3. AI service install, Ruff lint, and pytest.
4. Helm lint and Helm template render.
5. Gitleaks secret scan.
6. Dependency review for PRs.
7. CodeQL analysis for Go, JavaScript/TypeScript, and Python.
8. Trivy filesystem scan with SARIF upload.

Push or release tag gates:

1. Build `api`, `web`, and `ai-service` images.
2. Push images to Docker Hub.
3. Scan pushed images with Trivy.
4. Generate SPDX JSON SBOM artifacts.
5. Sign images with Cosign keyless signing through GitHub OIDC.

Dependabot is configured in [.github/dependabot.yml](.github/dependabot.yml) for Go modules, npm, pip, Docker, and GitHub Actions.

### Image Registry and Tags

The DevSecOps workflow publishes images under:

```text
docker.io/tuananh165/nexuschat-api
docker.io/tuananh165/nexuschat-web
docker.io/tuananh165/nexuschat-ai-service
```

Tagging behavior:

| Event | Image tag |
| --- | --- |
| Push to branch | Git SHA |
| Push tag `v*` | Release tag, for example `v0.1.0` |
| All pushed images | Also receive a `sha-*` metadata tag |

Production should use immutable release tags or SHA tags, not mutable `latest`.

Docker image references must be lowercase, so the Docker Hub namespace is configured as `tuananh165/*` in manifests and workflows.

### ArgoCD GitOps

Application manifests:

```text
deployments/gitops/applications
```

| File | Purpose |
| --- | --- |
| `nexuschat-staging.yaml` | Creates the `nexuschat` ArgoCD project and staging app |
| `nexuschat-production.yaml` | Production app pinned to a release tag |
| `platform-apps.yaml` | Platform apps for Nginx, kube-prometheus-stack, Consul, ECK/ELK, Kyverno |

Bootstrap order:

```bash
kubectl create namespace argocd
helm repo add argo https://argoproj.github.io/argo-helm
helm upgrade --install argocd argo/argo-cd \
  --namespace argocd \
  --values deployments/platform/argocd/values.yaml

kubectl apply -f deployments/gitops/applications/platform-apps.yaml
kubectl apply -f deployments/gitops/applications/nexuschat-staging.yaml
```

Apply production only after staging is healthy:

```bash
kubectl apply -f deployments/gitops/applications/nexuschat-production.yaml
```

Production changes should flow through Git commits and ArgoCD sync. Manual `kubectl apply` in production is reserved for emergency incident response and must be backfilled into Git.

### Platform Components

| Component | Path | Notes |
| --- | --- | --- |
| ArgoCD | `deployments/platform/argocd/values.yaml` | GitOps controller, ingress, HA settings |
| Nginx Ingress | `deployments/platform/ingress-nginx/values.yaml` | Metrics, JSON access logs, safer snippet defaults |
| Prometheus/Grafana | `deployments/platform/monitoring/kube-prometheus-stack-values.yaml` | Metrics, retention, Grafana persistence |
| Alerts | `deployments/platform/monitoring/nexuschat-rules.yaml` | 5xx, crash loop, scrape-down alerts |
| ELK/ECK | `deployments/platform/logging/eck-stack.yaml` | Elasticsearch, Kibana, Filebeat log shipping |
| Consul | `deployments/platform/consul/values.yaml` | Connect injection, TLS, ACLs, transparent proxy |
| Kyverno | `deployments/platform/security/kyverno-policies.yaml` | Non-root, no privilege escalation, resource requirements |

### Secrets and Configuration

Production secrets must be created outside Git. The Helm chart expects a secret named `nexuschat-runtime` in each application namespace through `global.envFromSecrets`.

Required secret keys include:

| Key | Used by |
| --- | --- |
| `CHAT_JWT_SECRET` | `chat` |
| `REDIS_PASSWORD` | Go services |
| `CASSANDRA_USER` | Go services |
| `CASSANDRA_PASSWORD` | Go services |
| `UPLOADER_S3_ACCESSKEY` | `uploader` |
| `UPLOADER_S3_SECRETKEY` | `uploader` |
| `USER_OAUTH_GOOGLE_CLIENTID` | `user` |
| `USER_OAUTH_GOOGLE_CLIENTSECRET` | `user` |
| `AI_POSTGRES_PASSWORD` | `ai-service` |
| AI provider credentials | `ai-service` |

Recommended production pattern:

1. Store source secrets in a cloud secret manager.
2. Install External Secrets Operator or a sealed-secrets workflow.
3. Sync the Kubernetes secret `nexuschat-runtime`.
4. Rotate secrets in the source secret manager.

### Security Baseline

Repository and build controls:

- Branch protection on `main`.
- Required PR reviews for application and DevSecOps-owned changes.
- Gitleaks for secret scanning.
- Dependency review for pull requests.
- CodeQL for Go, TypeScript, and Python.
- Trivy filesystem and image scans.
- SBOM artifacts generated for pushed images.
- Cosign keyless image signing through GitHub OIDC.

Runtime controls:

- Pods run as non-root.
- Containers drop Linux capabilities and disable privilege escalation.
- Read-only root filesystem is enabled in the application chart.
- Resource requests and limits are required.
- NetworkPolicy restricts ingress and egress to expected ports.
- Nginx ingress terminates TLS.
- Consul mesh provides service identity and internal traffic controls.
- Kyverno starts in audit mode and can be moved to enforce mode after clean releases.

### Observability

Metrics:

- Go services expose Prometheus metrics on port `8080`.
- `ai-service` exposes `/metrics` on port `8090`.
- `ServiceMonitor` resources are generated by the Helm chart when enabled.
- Nginx ingress and Consul metrics are enabled in platform values.

Logging:

- ECK-managed Elasticsearch and Kibana are scaffolded under `deployments/platform/logging`.
- Filebeat ships Kubernetes container logs.
- Create Kibana data views for `filebeat-*`.

Tracing:

- Local Docker Compose routes OpenTelemetry traces to Jaeger.
- Production should route OTLP to an OpenTelemetry Collector before long-term storage.

Alerts:

- High HTTP 5xx rate.
- Pod crash loops.
- Prometheus scrape target down.
- Add dependency-specific alerts for Kafka lag, Cassandra failures, Redis errors, AI provider failures, and upload storage errors before production cutover.

### Release Promotion

Standard release flow:

1. Merge to `main`.
2. Confirm CI, scans, SBOM, signing, and Helm validation pass.
3. Confirm staging ArgoCD sync is healthy.
4. Run staging smoke tests against the staging ingress domain.
5. Create a signed release tag:

```bash
git tag -s v0.1.0 -m "NexusChat v0.1.0"
git push origin v0.1.0
```

6. Update `deployments/gitops/applications/nexuschat-production.yaml` to the release tag and image tag.
7. Merge the promotion change.
8. Sync or approve production in ArgoCD.
9. Verify ingress, app health, metrics, logs, and error rate.

### Rollback

ArgoCD rollback:

```bash
argocd app history nexuschat-production
argocd app rollback nexuschat-production <revision-id>
```

GitOps image rollback:

1. Set `imageDefaults.tag` to the last known good immutable tag.
2. Commit the GitOps change.
3. Let ArgoCD sync production.
4. Confirm metrics, logs, and user flows recover.

## AI Service API

| Method | Endpoint | Description |
| --- | --- | --- |
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

## AI UI Flow

The AI controls are inside the chat composer.

Rewrite flow:

1. Open `http://localhost`.
2. Sign in and enter a chat room.
3. Type a draft message.
4. Click the `Sparkles` button next to the composer.
5. Choose `Professional`, `Friendly`, or `Shorter`.
6. Review the rewritten text and send it as a normal chat message.

Workflow preview flow:

1. Type a draft message.
2. Click the `Sparkles` button.
3. Choose `Tasks`, `Notes`, or `Checklist`.
4. Review the AI preview above the composer.

Direct AI service test:

```bash
curl -X POST http://localhost/api/ai/v1/assistant/rewrite \
  -H "Content-Type: application/json" \
  -d '{"text":"xin chao toi muon viet lai cau nay","tone":"professional","locale":"Vietnamese"}'
```

## Documentation

- [Engineering Docs Index](docs/README.md)
- [Architecture](docs/architecture.md)
- [AI Service Plan](docs/ai-service-plan.md)
- [DevSecOps Platform Plan](docs/devsecops-platform-plan.md)
- [DevSecOps Implementation Runbook](docs/devsecops-implementation-runbook.md)
- [Clean Code And Design Patterns](docs/clean-code-design-patterns.md)
- [AI Service README](ai-service/README.md)

## Production Readiness Checklist

- CI passes all test, scan, SBOM, signing, and Helm validation jobs.
- Images are immutable, signed, scanned, and promoted through GitOps.
- Runtime secrets are injected from approved secret storage, not committed.
- Nginx ingress TLS and DNS are configured for the target environment.
- Prometheus targets, Grafana dashboards, alerts, Kibana indexes, and Consul services are healthy.
- NetworkPolicy and Kyverno policies are reviewed before enforce mode.
- Staging smoke tests pass before production sync.
- Rollback target and previous image tags are known before deployment.

<p align="center">
  <sub>Built with Go | Python | Next.js | Kafka | Redis | Cassandra | PostgreSQL | MinIO | Kubernetes | Helm | ArgoCD | Nginx | Prometheus | Grafana | ELK | Consul</sub>
</p>
