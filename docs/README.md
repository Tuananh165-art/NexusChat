# NexusChat Documentation

This document describes the current NexusChat runtime: Go services, the Next.js frontend, the FastAPI `ai-service`, local Docker Compose, the Kubernetes Helm chart, and the GitHub Actions pipeline for direct deployment to the K3s lab.

## Main documentation

| File | Contents |
|---|---|
| [Project Reference](project-reference.md) | Complete business workflows, services, features, APIs, data ownership, migration policy, technology decisions, and limitations |
| [Architecture](architecture.md) | Service architecture, dependencies, Traefik, data flow, and observability |
| [Kubernetes/K3s Deployment Guide](deploy-k8s-guide.md) | Installing dependencies, deploying the lab Helm release, NodePort dashboards, verification, and rollback |
| [Docker Hub Direct K8s Rollout](dockerhub-direct-k8s-rollout.md) | Git SHA image process, security gates, and direct Helm CD |
| [DevSecOps Platform Plan](devsecops-platform-plan.md) | CI, CD, Trivy, CodeQL, Gitleaks, SBOM, Cosign, and current limitations |
| [AI Service Plan](ai-service-plan.md) | AI service design and implementation status |
| [Clean Code And Design Patterns](clean-code-design-patterns.md) | Coding and testing conventions |
| [Internal gRPC Security](internal-grpc-security.md) | Signed end-user assertions, mTLS configuration, key/certificate rotation, and failure behavior |
| [AI Service README](../ai-service/README.md) | How to run and configure the AI service |

## Source of truth by environment

### Local Docker Compose

Main commands:

```powershell
docker compose config
docker compose up --build -d
```

Files used directly by Compose:

| File | Role |
|---|---|
| `docker-compose.yaml` | Defines the local app, database, broker, cache, storage, Traefik, and observability |
| `deployments/prometheus/prometheus.yaml` | Static scrape targets for Compose |
| `deployments/grafana/provisioning/datasources/datasources.yaml` | Prometheus and Jaeger data sources for local Grafana |
| `deployments/otel-collector/config.yaml` | OTLP receiver and trace export to local Jaeger |

Docker Compose uses host ports, not Kubernetes NodePorts. Local Grafana is `http://localhost:3000`, Prometheus is `http://localhost:9090`, and Jaeger is `http://localhost:16686`.

### Kubernetes/K3s

The main application deployment file is not `values-lab-k3s.yaml` by itself. The Helm chart is assembled in this order:

```text
deployments/helm/nexuschat/Chart.yaml
+ deployments/helm/nexuschat/values.yaml
+ deployments/helm/nexuschat/values-lab-k3s.yaml
+ deployments/helm/nexuschat/templates/*
```

`values.yaml` is the base configuration; `values-lab-k3s.yaml` is the override for the 4GB/50GB lab. When both are passed to Helm, the lab file overrides base values. The workflow uses this exact pair of files.

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-k3s.yaml \
  --set-string imageDefaults.tag="$GIT_SHA" \
  --wait --timeout 10m
```

The current lab profile uses Traefik, host `nexuschat.click`, HTTP-only, and `tlsSecretName: ""`. The domain must resolve to the node/Ingress IP; Helm does not create DNS or TLS automatically.

## Kubernetes platform manifests

Platform manifests are not automatically rendered by the application Helm chart. They must be installed separately or applied by the pipeline:

| Path | Used? | Role |
|---|---|---|
| `deployments/platform/observability/` | Yes | Jaeger and OpenTelemetry Collector for Kubernetes |
| `deployments/platform/dashboards/` | Yes | Kafka UI and RedisInsight NodePorts |
| `deployments/platform/monitoring/kube-prometheus-stack-values.yaml` | When installing kube-prometheus-stack | Grafana NodePort `30300`, Prometheus `30900`, and lightweight lab profile |
| `deployments/platform/monitoring/ingresses.yaml` | Optional | Traefik Ingress for Grafana, Prometheus, and Jaeger |
| `deployments/platform/ingresses.yaml` | Optional | Traefik Ingress for Kafka UI, RedisInsight, and MinIO |
| `deployments/platform/security/kyverno-policies.yaml` | If using Kyverno | Runtime policy; Kyverno must be installed first |
| `deployments/platform/cassandra/cassandra.yaml` | Optional | Standalone lab Cassandra |
| `deployments/run.sh` | Local helper | Legacy Docker Compose helper |

When installing kube-prometheus-stack, create the `grafana-admin` Secret in the `monitoring` namespace first because the values do not contain credentials:

```bash
kubectl -n monitoring create secret generic grafana-admin \
  --from-literal=admin-user=admin \
  --from-literal=admin-password='<strong-random-password>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

Install kube-prometheus-stack with the lab profile:

```bash
helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace \
  --values deployments/platform/monitoring/kube-prometheus-stack-values.yaml \
  --wait --timeout 10m
```

Apply the observability/dashboard manifests separately:

```bash
kubectl apply -f deployments/platform/observability
kubectl apply -f deployments/platform/dashboards
```


| Dashboard | URL |
|---|---|
| Kafka UI | `http://<NODE_IP>:30080` |
| RedisInsight | `http://<NODE_IP>:30540` |
| Grafana | `http://<NODE_IP>:30300` |
| Prometheus | `http://<NODE_IP>:30900` |
| Jaeger | `http://<NODE_IP>:30686` |

NodePort provides direct access to the node and does not pass through Traefik. If `nexuschat.click` points to the node IP and the firewall permits the port, a URL such as `http://nexuschat.click:30300` may work; however, this is not Traefik TLS/domain routing. For host routing through Traefik, configure DNS for `grafana.nexuschat.click`, `prometheus.nexuschat.click`, `jaeger.nexuschat.click`, `kafka.nexuschat.click`, and `redis.nexuschat.click`, then apply `deployments/platform/monitoring/ingresses.yaml` and `deployments/platform/ingresses.yaml`. These Ingresses do not create DNS or TLS automatically.

## CI/CD source of truth

The current workflow is `.github/workflows/devsecops-platform.yml`.

The workflow triggers on `workflow_dispatch`, `pull_request` to any branch, pushes to `main` or `kafka`, and `v*` tags. The Kubernetes deploy job runs after the required image jobs succeed for either a push to `main` or a manual `workflow_dispatch`; it requires `build-images` and `build-proxy-variants` to pass, plus a self-hosted runner with labels `[self-hosted, linux, x64, k3s-lab]`.

- Pull Request: tests, lint, builds, Helm rendering, and security scans.
- Push to `main`, `kafka`, or a `v*` tag: build images, blocking Trivy scans, SBOM generation, and Cosign signing.
- Push to `main`: apply Jaeger/OTel and dashboard manifests, then directly deploy the app with Helm into `nexuschat-lab`.

Stateful dependencies such as Kafka, Redis, Cassandra, MinIO, and PostgreSQL are not installed automatically by the application Helm chart; install them separately or use managed/external services, then update the hostnames in the values. The workflow also does not automatically provision kube-prometheus-stack or Kyverno. A `git pull` alone does not deploy; `git commit` followed by `git push` to `main` can trigger CD.

## Generated API docs

The following directories are generated from Swagger annotations; do not edit them manually when updating the API:

- `docs/user`
- `docs/match`
- `docs/chat`
- `docs/uploader`

Regenerate them with:

```bash
make doc
```

