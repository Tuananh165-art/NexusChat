# NexusChat Engineering Docs

Tài liệu trong thư mục này mô tả đúng trạng thái source hiện tại của NexusChat: Go microservices, Next.js frontend được serve bởi Go `web`, Python FastAPI `ai-service`, Docker Compose local, Helm chart Kubernetes, và pipeline GitHub Actions -> Docker Hub -> K3s lab.

## Tài liệu chính

| File | Nội dung |
| --- | --- |
| [Architecture](architecture.md) | Kiến trúc runtime, service boundary, data ownership, API/ingress/dependency flow |
| [K3s/Kubernetes Deployment Guide](deploy-k8s-guide.md) | Runbook lab 4GB, dependencies, Helm deploy, GitHub Actions CD, verify/rollback |
| [Docker Hub + Direct K8s Rollout](dockerhub-argocd-rollout-vi.md) | Hướng dẫn tiếng Việt ngắn gọn cho pipeline Docker Hub + self-hosted runner + Helm |
| [DevSecOps Platform Plan](devsecops-platform-plan.md) | Mô hình môi trường, CI/CD, security gates, observability, release promotion |
| [DevSecOps Implementation Runbook](devsecops-implementation-runbook.md) | Checklist triển khai/operating hiện tại, có phân biệt lab trực tiếp và GitOps optional |
| [AI Service Plan](ai-service-plan.md) | Thiết kế và trạng thái implement của Python AI service |
| [Clean Code And Design Patterns](clean-code-design-patterns.md) | Quy tắc code Go/Next.js/Python AI service và kỳ vọng test |
| [AI Service README](../ai-service/README.md) | Cách chạy và API của `ai-service` |

## Generated API docs

Các thư mục sau được sinh từ Swagger annotations Go, không sửa tay nếu chỉ muốn cập nhật API docs:

- `docs/user`
- `docs/match`
- `docs/chat`
- `docs/uploader`

Sinh lại bằng:

```bash
make doc
```

## Kubernetes assets liên quan

| Path | Vai trò hiện tại |
| --- | --- |
| `deployments/helm/nexuschat` | Helm chart application cho `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, `ai-service` |
| `deployments/helm/nexuschat/values.yaml` | Default staging-like profile, nginx ingress, Consul annotation enabled by default, ServiceMonitor/NetworkPolicy enabled |
| `deployments/helm/nexuschat/values-lab-4gb.yaml` | Lab profile 4GB: one replica, Recreate, nginx ingress, no Consul, no ServiceMonitor, no NetworkPolicy |
| `deployments/gitops/applications/*.yaml` | Optional ArgoCD app definitions; current lab CD does not depend on ArgoCD |
| `deployments/platform` | Optional platform/lab dashboard manifests: ingress-nginx values, monitoring/logging/security/Consul/ArgoCD references |

## CI/CD source of truth

Workflow hiện tại: `.github/workflows/devsecops-platform.yml`.

- PR: test/lint/build/scan/template only.
- Push `main`/`kafka`/tag `v*`: build, push, scan, SBOM, sign Docker Hub images.
- Push `main`: deploy lab K3s namespace `nexuschat-lab` by direct `helm upgrade --install` on self-hosted runner labels `[self-hosted, linux, x64, k3s-lab]`.

Chi tiết deploy nằm trong `deploy-k8s-guide.md` và `dockerhub-argocd-rollout-vi.md`.
