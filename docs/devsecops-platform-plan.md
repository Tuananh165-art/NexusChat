# NexusChat DevSecOps Platform Plan

This plan defines the production-grade DevSecOps baseline for running NexusChat on Kubernetes with Helm, GitHub Actions, Nginx Ingress, Prometheus/Grafana, ELK, ArgoCD GitOps, and Consul service mesh.

## Target Architecture

- Kubernetes is the runtime control plane for stateless NexusChat services and stateful dependencies.
- Helm owns deployable application configuration and platform component values.
- GitHub Actions owns CI, security checks, image publication, SBOM generation, signing, and chart validation.
- ArgoCD owns continuous delivery from Git to cluster.
- Nginx Ingress is the north-south HTTP entry point.
- Consul service mesh owns east-west service identity, transparent proxying, and service intentions.
- Prometheus/Grafana owns metrics, dashboards, alerts, and SLO visibility.
- ELK owns centralized application, ingress, audit, and platform logs.
- Security controls are applied at repository, build, image, cluster, network, and runtime layers.

## Environment Model

| Environment | Branch or tag | Namespace | Delivery mode | Purpose |
| --- | --- | --- | --- | --- |
| `dev` | feature branches and PRs | preview namespaces | CI validation only | Fast feedback, no production data |
| `staging` | `main` | `nexuschat-staging` | ArgoCD auto-sync | Release rehearsal and integration tests |
| `production` | signed `v*` tags | `nexuschat-prod` | ArgoCD sync with manual promotion | Customer-facing runtime |

## Kubernetes Baseline

- Use one namespace per environment.
- Run all NexusChat pods as non-root with read-only root filesystems where supported.
- Apply resource requests and limits for every container.
- Use horizontal pod autoscaling for HTTP-facing services.
- Use pod disruption budgets for services with more than one replica.
- Use NetworkPolicy default-deny semantics and explicitly allow ingress, mesh, observability, and dependency traffic.
- Store runtime secrets outside Git. The chart supports `envFromSecrets`; production should bind those through External Secrets Operator or sealed secrets.
- Use TLS at the ingress boundary. Internal service-to-service encryption and identity are delegated to Consul mesh.

## Helm Scope

The application chart under `deployments/helm/nexuschat` manages:

- `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, and `ai-service` deployments.
- Services and ingress routes.
- Prometheus scrape annotations and optional `ServiceMonitor` resources.
- Service accounts, security contexts, resource limits, HPAs, PDBs, and NetworkPolicies.
- Consul mesh annotations and service defaults for transparent proxy.

Stateful dependencies such as Kafka, Redis, Cassandra, MinIO, and Postgres should be deployed with dedicated vendor charts or managed services. They are intentionally not embedded in the application chart.

## CI/CD Flow

1. Pull request opens.
2. GitHub Actions runs Go tests, frontend tests/build, AI service tests, lint, secret scan, dependency review, CodeQL, Trivy filesystem scan, and Helm lint/template.
3. Merge to `main` builds images for `api`, `web`, and `ai-service`, publishes to the container registry, generates SBOMs, signs images, and optionally updates staging Helm values.
4. ArgoCD detects the Git change and syncs staging.
5. Release tag `v*` builds immutable release images and updates production values through a controlled pull request or manual GitOps promotion.

## Security Gates

- Secret scanning: Gitleaks.
- Dependency review: GitHub dependency review for pull requests.
- Static analysis: CodeQL for Go, JavaScript/TypeScript, and Python.
- Container and filesystem scanning: Trivy.
- SBOM: Syft SPDX JSON artifacts.
- Image signing: Cosign keyless signing through GitHub OIDC.
- Kubernetes policy validation: Conftest/Kyverno or admission controller in cluster.
- Runtime security: least privilege security contexts, NetworkPolicy, Consul intentions, and ingress rate limiting.

## Observability

- Metrics: Prometheus Operator scrapes `/metrics` on port `8080` for Go services and `/metrics` on `8090` for AI service.
- Dashboards: Grafana loads application, ingress, Consul, Kubernetes, and dependency dashboards.
- Alerts: SLO burn rate, pod crash loops, high latency, elevated 5xx, queue lag, Cassandra errors, Redis errors, and AI provider failures.
- Traces: services emit OTLP traces; production should route OTLP to a supported collector before long-term storage.
- Logs: Filebeat or Elastic Agent ships container logs to Elasticsearch. Kibana owns search and operational views.

## GitOps Operating Rules

- Cluster state is changed by Git commits and ArgoCD syncs, not by manual `kubectl apply` in production.
- Emergency hotfixes must be backfilled into Git within the same incident.
- Application chart changes must pass Helm lint/template in CI.
- Platform chart values must be reviewed by DevSecOps owners before merge.
- Production ArgoCD apps must use immutable image tags and restricted sync windows.

## Rollout Checklist

1. Create namespaces: `argocd`, `ingress-nginx`, `consul`, `monitoring`, `logging`, `security`, `nexuschat-staging`, `nexuschat-prod`.
2. Install platform components using the values in `deployments/platform`.
3. Configure DNS and TLS issuer for the Nginx ingress controller.
4. Configure image registry credentials and External Secrets or Sealed Secrets.
5. Register ArgoCD applications from `deployments/gitops/applications`.
6. Verify Prometheus targets, Grafana dashboards, Kibana indexes, Consul services, and Nginx routes.
7. Run staging smoke tests before production promotion.

## Production Readiness Definition

- CI passes all tests and security gates.
- Images are immutable, signed, scanned, and have SBOM artifacts.
- Helm manifests render successfully for the target environment.
- Ingress TLS, authentication-sensitive cookies, and CORS settings match the environment domain.
- Secrets are injected from approved secret storage.
- Service mesh intentions allow only required service-to-service paths.
- Prometheus alerts and dashboards are active before traffic cutover.
- Rollback command and previous image tags are documented for the release.
