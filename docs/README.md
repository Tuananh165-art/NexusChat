# NexusChat Engineering Docs

This directory contains generated API documentation and repo-native engineering guidance.

## Core Documents

- [Architecture](architecture.md): service boundaries, data ownership, and runtime dependencies.
- [Clean Code And Design Patterns](clean-code-design-patterns.md): project-wide implementation rules for Go backend and Next.js frontend code.
- [AI Service Plan](ai-service-plan.md): phased architecture and implementation plan for the independent Python AI service.
- [DevSecOps Platform Plan](devsecops-platform-plan.md): Kubernetes, Helm, GitHub Actions, security, Nginx, Prometheus/Grafana, ELK, ArgoCD, and Consul operating model.
- [DevSecOps Implementation Runbook](devsecops-implementation-runbook.md): step-by-step cluster bootstrap, release promotion, observability, logging, mesh, admission, and rollback procedure.
- [Server Kubernetes Deployment Guide](server-kubernetes-deployment-guide.md): end-to-end commands for installing Kubernetes on a server and deploying NexusChat with the repo's DevSecOps assets.
- [Server Kubernetes Lab 4GB Guide](server-kubernetes-lab-4gb-guide.md): minimal lab deployment path for a 4GB RAM / 50GB disk server.

## Deployment Assets

- `deployments/helm/nexuschat`: application Helm chart for NexusChat services.
- `deployments/helm/nexuschat/values-lab-4gb.yaml`: minimal Helm override for small lab servers.
- `deployments/gitops/applications`: ArgoCD application manifests for staging, production, and platform components.
- `deployments/platform`: baseline Helm values and policies for ingress, monitoring, logging, Consul mesh, ArgoCD, and security admission.

## Generated API Docs

- `docs/user`
- `docs/match`
- `docs/chat`
- `docs/uploader`

Generated Swagger files should be updated through the existing Make targets instead of hand editing.
