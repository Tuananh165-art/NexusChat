# NexusChat

NexusChat is a Go/Next.js real-time chat platform for anonymous matching. It includes Go services, a Next.js frontend served by `web`, and the FastAPI `ai-service`. The current lab runtime uses HTTP and WebSocket (`ws://`) through Traefik. See the complete [project reference](docs/project-reference.md) for business workflows, service boundaries, feature/API inventory, migration policy, and known limitations.

## Architecture

```text
Browser
  │ HTTP + WebSocket
  ▼
Traefik
  ├── web ── chat ── match
  │            │       ├── discovery
  │            │       └── safety
  │            ├── safety before broadcast
  │            └── workspace
  └── ai-service ── PostgreSQL

Notification intents ── Cassandra outbox ── notification worker

Go services ── Kafka + Redis Cluster + Cassandra
Traces ── OpenTelemetry Collector ── Jaeger
Metrics ── Prometheus ── Grafana
```

## Services

| Service | Command/Image | Function |
|---|---|---|
| web | `server web` / `nexuschat-web` | Static Next.js and HTTP/WebSocket gateway |
| chat | `server chat` / `nexuschat-api` | Chat, channels, and broadcasting |
| match | `server match` / `nexuschat-api` | Matching and waitlist |
| user | `server user` / `nexuschat-api` | Users, OAuth, and sessions |
| notification | `server notification` / `nexuschat-api` | Durable notification outbox/retry worker |
| uploader | `server uploader` / `nexuschat-api` | File uploads through MinIO |
| forwarder | `server forwarder` / `nexuschat-api` | Kafka fanout/routing |
| safety | `server safety` / `nexuschat-safety` | Moderation, reporting, blocking, and risk scoring |
| discovery | `server discovery` / `nexuschat-discovery` | Interest profiles and ranking |
| workspace | `server workspace` / `nexuschat-workspace` | Tasks, notes, bookmarks, Kanban, and reminders |
| ai-service | `uvicorn app.main:app` / `nexuschat-ai-service` | AI workflows and semantic enrichment |

## Local Docker Compose

Requirements: Go 1.24, Node.js 20+, Docker Desktop, and the dependencies in Compose.

```powershell
Copy-Item .env.example .env
docker compose config
docker compose up --build -d
docker compose ps
```

Compose starts the app, Traefik, AI PostgreSQL, Redis Cluster, MinIO, Kafka, Cassandra, Prometheus, Grafana, Jaeger, and the OpenTelemetry Collector. `createbucket` and `cassandra-init` are one-shot jobs: they complete successfully and may be rerun; bucket creation is idempotent.

Local URLs:

| Component | URL |
|---|---|
| App/Traefik | `http://localhost` |
| Traefik dashboard | `http://localhost:8080` |
| MinIO API/Console | `http://localhost:9000` / `http://localhost:9001` |
| Prometheus | `http://localhost:9090` |
| Grafana | `http://localhost:3000` |
| Jaeger | `http://localhost:16686` |

Compose observability configuration:

- `deployments/prometheus/prometheus.yaml`: scrape configuration.
- `deployments/grafana/provisioning/datasources/datasources.yaml`: Prometheus/Jaeger data sources.
- `deployments/otel-collector/config.yaml`: OTLP receiver and Jaeger exporter.

Docker Compose uses host ports, **not Kubernetes NodePorts**. Kafka/Redis do not have an integrated web dashboard in Compose.

## Database, migrations, and events

Cassandra has two related but different inputs. `cassandra/init.cql` is an idempotent standalone/bootstrap schema. The versioned files under `cassandra/migrations/` are the upgrade ledger and must not be ignored. Compose runs `init.cql` and then every unapplied migration in lexical order. Helm runs the pre-install/pre-upgrade migration Job with the chart-embedded `001_baseline.cql` through `010_*.cql`; migration `010_drop_messages_seen.cql` removes the obsolete per-message flag after old binaries are retired. When the Job succeeds, do not manually apply `init.cql` or individual migrations. Back up production data before upgrades.


Cassandra stores messages/channels, safety decisions, discovery profiles, workspace data, notification intents, and delivered notifications. The `notification` process claims leased intents, retries delivery with exponential backoff, and marks exhausted intents as dead-letter (`dead`). User-facing notification creation only enqueues an intent; the worker performs the durable write asynchronously.
- Redis Cluster provides caching, rate limiting, locking, waitlists, reminder leases, and deduplication.
- Kafka uses the topics `nexuschat.chat.events.v1`, `nexuschat.safety.events.v1`, `nexuschat.discovery.events.v1`, and `nexuschat.workspace.events.v1`.
- PostgreSQL is used by the AI service/Alembic.
- MinIO stores uploaded objects in the `myfilebucket` bucket.

Consumers use versioned envelopes, `processed_events` idempotency, an outbox relay, retry backoff, and a DLQ.

## Kubernetes/K3s deployment

### Relationship between values files

The main deployment file is the Helm chart:

```text
deployments/helm/nexuschat/
├── Chart.yaml
├── values.yaml                 # base values
├── values-lab-k3s.yaml         # override lab
└── templates/                  # Deployment, Service, Ingress, HPA, policy...
```

`values-lab-k3s.yaml` **is not an independent deployment manifest**. The command must pass both values files in order:

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-k3s.yaml \
  --set-string imageDefaults.tag="$GIT_SHA" \
  --wait --timeout 10m
```

The lab file currently configures:

- `global.environment: lab`.
- Application namespace `nexuschat-lab`.
- Traefik `ingressClassName: traefik`.
- Host `nexuschat.click`.
- HTTP-only: `global.tlsSecretName: ""`, with cert-manager disabled.
- One replica and `Recreate` for the small lab environment.
- App ServiceMonitor/NetworkPolicy disabled.

The app Helm chart does not install stateful dependencies. Kafka, Redis, Cassandra, MinIO, PostgreSQL, Prometheus/Grafana, Jaeger/OTel, Traefik, and Kyverno must be prepared separately.

### Platform files used

| Path | Usage |
|---|---|
| `deployments/platform/observability/` | `kubectl apply` Jaeger and OpenTelemetry Collector |
| `deployments/platform/dashboards/` | `kubectl apply` Kafka UI and RedisInsight |
| `deployments/platform/monitoring/kube-prometheus-stack-values.yaml` | Values for installing kube-prometheus-stack; Grafana/Prometheus NodePorts and lightweight lab profile |
| `deployments/platform/monitoring/ingresses.yaml` | Optional Traefik Ingress for Grafana, Prometheus, and Jaeger |
| `deployments/platform/ingresses.yaml` | Optional Traefik Ingress for Kafka UI, RedisInsight, and MinIO |
| `deployments/platform/security/kyverno-policies.yaml` | Apply after installing Kyverno |
| `deployments/platform/cassandra/cassandra.yaml` | Optional standalone lab Cassandra |
| `deployments/run.sh` | Legacy/local Docker Compose helper |

### Installing observability, monitoring, dashboards, and policy

Create the Grafana Secret before installing kube-prometheus-stack; do not put a real password in Git:

```bash
kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -
kubectl -n monitoring create secret generic grafana-admin \
  --from-literal=admin-user=admin \
  --from-literal=admin-password='<strong-random-password>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

```bash
kubectl apply -f deployments/platform/observability
kubectl apply -f deployments/platform/dashboards
```

Install kube-prometheus-stack:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --values deployments/platform/monitoring/kube-prometheus-stack-values.yaml \
  --wait --timeout 10m
```

Install Kyverno and the policy:

```bash
helm repo add kyverno https://kyverno.github.io/kyverno/
helm repo update
helm upgrade --install kyverno kyverno/kyverno \
  --namespace security --create-namespace --wait --timeout 10m
kubectl apply -f deployments/platform/security/kyverno-policies.yaml
```

### NodePort dashboards

| Dashboard | NodePort | Direct URL |
|---|---:|---|
| Kafka UI | `30080` | `http://<NODE_IP>:30080` |
| RedisInsight | `30540` | `http://<NODE_IP>:30540` |
| Grafana | `30300` | `http://<NODE_IP>:30300` |
| Prometheus | `30900` | `http://<NODE_IP>:30900` |
| Jaeger | `30686` | `http://<NODE_IP>:30686` |

`nexuschat.click:<port>` can be used instead of `<NODE_IP>` if DNS `nexuschat.click` points to the node IP and the firewall allows the port. This is a direct NodePort, bypasses Traefik, and does not provide TLS automatically. Production should use subdomains through Traefik, such as `grafana.nexuschat.click`, `prometheus.nexuschat.click`, and `jaeger.nexuschat.click`, with authentication/VPN/TLS.

### Deployment checks

```bash
kubectl get nodes -o wide
kubectl get ingressclass
kubectl -n nexuschat-lab get pods,svc,ingress
kubectl -n monitoring get pods,svc
kubectl -n kafka get pods,svc
kubectl -n redis-ui get pods,svc

for deploy in web chat match user notification uploader forwarder ai-service safety discovery workspace; do
  kubectl -n nexuschat-lab rollout status deployment/$deploy --timeout=600s
done
```

App:

```bash
curl -i http://nexuschat.click
```

The lab disables the public AI ingress. To check AI health, forward the internal Service in one terminal:

```bash
kubectl -n nexuschat-lab port-forward svc/ai-service 18090:8090
```

Then, in a second terminal:

```bash
curl -i http://127.0.0.1:18090/health
```

Dashboard:

```bash
curl -I http://<NODE_IP>:30300
curl -I http://<NODE_IP>:30900
curl -I http://<NODE_IP>:30686
curl -I http://<NODE_IP>:30080
curl -I http://<NODE_IP>:30540
```

## CI/CD and security

Workflow source of truth: `.github/workflows/devsecops-platform.yml`.

The workflow triggers on `workflow_dispatch`, `pull_request` to any branch, pushes to `main` or `kafka`, and `v*` tags. The Kubernetes deploy job runs after the required image jobs succeed for either a push to `main` or a manual `workflow_dispatch`; it requires `build-images` and `build-proxy-variants` to pass, plus a self-hosted runner with labels `[self-hosted, linux, x64, k3s-lab]`.

- Pull Request: Go tests/race tests, frontend lint/build, AI lint/tests, Helm render, Gitleaks, Dependency Review, CodeQL, and Trivy filesystem scanning.
- Push to `main`, `kafka`, or a `v*` tag: build images, perform blocking Trivy scans for HIGH/CRITICAL findings, generate SPDX SBOMs, and use Cosign.
- Proxy variants are also scanned, included in SBOMs, and signed; they are pushed only after the security gate passes.
- Push to `main`: apply Jaeger/OTel and dashboard manifests, then perform a direct Helm deployment of the app to the K3s lab.

CI/CD does not automatically provision Kafka, Redis, Cassandra, MinIO, PostgreSQL, Traefik, kube-prometheus-stack, or Kyverno. These dependencies must be Ready before CD. A `git pull` alone does not deploy; `git commit` followed by `git push` to `main` can trigger CD.

## Development

Backend:

```powershell
go run . chat
go run . match
go run . safety
go run . discovery
go run . workspace
go run . notification
```

Frontend:

```powershell
cd frontend
npm.cmd ci
npm.cmd run dev
```

Build/test:

```powershell
go test ./...
go test -race ./pkg/realtime ./pkg/safety ./pkg/discovery ./pkg/workspace ./pkg/chat ./pkg/match
cd frontend
npm.cmd run build
cd ..
docker compose config
```

See also [docs/README.md](docs/README.md) and [docs/deploy-k8s-guide.md](docs/deploy-k8s-guide.md).

