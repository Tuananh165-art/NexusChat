# NexusChat Engineering Docs

This directory documents the current NexusChat source: Go microservices, a Next.js frontend served by Go `web`, a Python FastAPI `ai-service`, local Docker Compose, the Kubernetes Helm chart, and the GitHub Actions -> Docker Hub -> K3s lab pipeline.

## Main documents

| File | Content |
| --- | --- |
| [Architecture](architecture.md) | Runtime architecture, service boundaries, data ownership, API/ingress/dependency flow |
| [K3s/Kubernetes Deployment Guide](deploy-k8s-guide.md) | 4GB lab runbook, dependencies, Helm deploy, GitHub Actions CD, verification, and rollback |
| [Docker Hub + Direct K8s Rollout](dockerhub-direct-k8s-rollout.md) | Concise CI/CD guide for Docker Hub + self-hosted runner + Helm |
| [DevSecOps Platform Plan](devsecops-platform-plan.md) | Environment model, CI/CD, security gates, observability, and release promotion |
| [DevSecOps Implementation Runbook](devsecops-implementation-runbook.md) | Current implementation/operations checklist with direct lab deployment and optional GitOps references |
| [AI Service Plan](ai-service-plan.md) | Python AI service design and implementation state |
| [Clean Code And Design Patterns](clean-code-design-patterns.md) | Go/Next.js/Python AI service coding standards and testing expectations |
| [AI Service README](../ai-service/README.md) | How to run `ai-service` and its API |

## Generated API docs

The following directories are generated from Go Swagger annotations. Do not edit them by hand when the goal is to update API docs:

- `docs/user`
- `docs/match`
- `docs/chat`
- `docs/uploader`

Regenerate them with:

```bash
make doc
```

## Related Kubernetes assets

| Path | Current role |
| --- | --- |
| `deployments/helm/nexuschat` | Application Helm chart for `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, and `ai-service` |
| `deployments/helm/nexuschat/values.yaml` | Default staging-like profile, nginx ingress, Consul annotation enabled by default, ServiceMonitor/NetworkPolicy enabled |
| `deployments/helm/nexuschat/values-lab-4gb.yaml` | 4GB lab profile: one replica, Recreate rollout, nginx ingress, no Consul, no ServiceMonitor, no NetworkPolicy |
| `deployments/gitops/applications/*.yaml` | Optional ArgoCD app definitions; current lab CD does not depend on ArgoCD |
| `deployments/platform` | Optional platform/lab dashboard manifests: ingress-nginx values, monitoring/logging/security/Consul/ArgoCD references |

## CI/CD source of truth

Current workflow: `.github/workflows/devsecops-platform.yml`.

- PR: test/lint/build/scan/template only.
- Push `main`/`kafka`/tag `v*`: build, push, scan, SBOM, and sign Docker Hub images.
- Push `main`: deploy lab K3s namespace `nexuschat-lab` by direct `helm upgrade --install` on self-hosted runner labels `[self-hosted, linux, x64, k3s-lab]`.

Deployment details are in `deploy-k8s-guide.md` and `dockerhub-direct-k8s-rollout.md`.
