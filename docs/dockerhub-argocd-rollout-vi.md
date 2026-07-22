# Hướng dẫn CI/CD NexusChat: GitHub Actions + Docker Hub + deploy K8s trực tiếp

Tài liệu này mô tả đúng flow hiện tại của source code. Tên file còn chữ `argocd` vì lịch sử, nhưng lab CD hiện tại không phụ thuộc ArgoCD. ArgoCD chỉ còn là manifest optional/reference trong `deployments/gitops/applications`.

## 1. Flow chuẩn hiện tại

Khi code được push lên `main`:

1. GitHub Actions chạy validation: Go test, frontend lint/build, AI service lint/test, Helm lint/template, security scans.
2. Build image chính và push lên Docker Hub.
3. Build thêm image variant cho lab/proxy upload.
4. Scan image, tạo SBOM, ký image bằng Cosign keyless.
5. Job `deploy-lab-k8s` chạy trên self-hosted runner có label `k3s-lab`.
6. Job dùng kubeconfig từ `KUBE_CONFIG_B64` nếu có, hoặc kubeconfig sẵn trên runner.
7. Chạy `helm upgrade --install` vào namespace `nexuschat-lab`.
8. Chờ rollout các Deployment: `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, `ai-service`.
9. In image thực tế đang chạy trong cluster.

## 2. File chính

| File | Vai trò |
| --- | --- |
| `.github/workflows/devsecops-platform.yml` | Pipeline CI/CD chính |
| `deployments/helm/nexuschat` | Helm chart application |
| `deployments/helm/nexuschat/values.yaml` | Values mặc định |
| `deployments/helm/nexuschat/values-lab-4gb.yaml` | Override cho lab K3s nhỏ |
| `docs/deploy-k8s-guide.md` | Runbook triển khai lab chi tiết |

## 3. Image được build/push

Image chính:

- `docker.io/tuananh165/nexuschat-api:<TAG>`
- `docker.io/tuananh165/nexuschat-web:<TAG>`
- `docker.io/tuananh165/nexuschat-ai-service:<TAG>`

Variant phục vụ lab/proxy upload:

- `docker.io/tuananh165/nexuschat-api:proxy-upload`
- `docker.io/tuananh165/nexuschat-api:proxy-upload-<TAG>`
- `docker.io/tuananh165/nexuschat-web:proxy-upload`
- `docker.io/tuananh165/nexuschat-web:proxy-upload-<TAG>`
- `docker.io/tuananh165/nexuschat-web:proxy-upload-v2`
- `docker.io/tuananh165/nexuschat-web:proxy-upload-v2-<TAG>`

`TAG` là:

- `GITHUB_SHA` nếu push branch.
- `GITHUB_REF_NAME` nếu push tag `v*`.

## 4. GitHub Actions secrets bắt buộc

Tạo trong GitHub repository: `Settings` -> `Secrets and variables` -> `Actions`.

| Secret | Mục đích |
| --- | --- |
| `DOCKER_USERNAME` | Login Docker Hub |
| `DOCKER_PASSWORD` | Docker Hub access token/password |
| `KUBE_CONFIG_B64` | Kubeconfig base64 cho lab deploy, có thể bỏ trống nếu self-hosted runner đã có kubeconfig |

Tạo `KUBE_CONFIG_B64`:

```bash
base64 -w0 ~/.kube/config
```

Lab hiện tại nên dùng self-hosted runner trong cùng mạng với K3s. Workflow đang yêu cầu labels:

```text
self-hosted, linux, x64, k3s-lab
```

## 5. Command deploy mà workflow đang chạy

Trong job `deploy-lab-k8s`, Helm command hiện tại tương đương:

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

Mapping image thực tế:

| Service | Image lab |
| --- | --- |
| `web` | `docker.io/tuananh165/nexuschat-web:proxy-upload-v2-<SHA>` |
| `uploader` | `docker.io/tuananh165/nexuschat-api:proxy-upload-<SHA>` |
| `chat` | `docker.io/tuananh165/nexuschat-api:<SHA>` |
| `match` | `docker.io/tuananh165/nexuschat-api:<SHA>` |
| `user` | `docker.io/tuananh165/nexuschat-api:<SHA>` |
| `forwarder` | `docker.io/tuananh165/nexuschat-api:<SHA>` |
| `ai-service` | `docker.io/tuananh165/nexuschat-ai-service:<SHA>` |

## 6. Kiểm tra sau khi push

Xem workflow:

```bash
gh run list --workflow "DevSecOps Platform Pipeline" --limit 5
gh run watch
```

Kiểm tra cluster:

```bash
kubectl -n nexuschat-lab get pods -o wide
kubectl -n nexuschat-lab get deploy
kubectl -n nexuschat-lab get ingress
kubectl -n nexuschat-lab get deploy -o jsonpath='{range .items[*]}{.metadata.name}{" => "}{range .spec.template.spec.containers[*]}{.image}{" "}{end}{"\n"}{end}'
```

Rollout status đúng theo deployment names hiện tại:

```bash
for deploy in web chat match user uploader forwarder ai-service; do
  kubectl -n nexuschat-lab rollout status deployment/$deploy --timeout=600s
done
```

Smoke test lab IP hiện tại:

```bash
curl -I http://192.168.109.131
curl -i http://192.168.109.131/api/ai/health
```

## 7. Rollback nhanh

```bash
helm history nexuschat -n nexuschat-lab
helm rollback nexuschat <REVISION> -n nexuschat-lab
```

Hoặc redeploy tag cũ:

```bash
export OLD_TAG='<old-git-sha>'
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-4gb.yaml \
  --set-string imageDefaults.tag="$OLD_TAG" \
  --set-string services.web.image.fullname="docker.io/tuananh165/nexuschat-web:proxy-upload-v2-$OLD_TAG" \
  --set-string services.uploader.image.fullname="docker.io/tuananh165/nexuschat-api:proxy-upload-$OLD_TAG" \
  --wait --timeout 10m
```

## 8. Lệnh commit/push khuyến nghị

Vì working tree hiện tại có nhiều file modified/untracked, không nên `git add .` nếu chưa review toàn bộ. Với riêng thay đổi docs:

```bash
git status --short
git diff -- README.md docs/*.md ai-service/README.md deployments/platform/UI-ACCESS.md
git add README.md docs/*.md ai-service/README.md deployments/platform/UI-ACCESS.md
git commit -m "docs: update architecture and k8s deployment pipeline"
git push origin main
```

Chỉ dùng `git add .` sau khi đã chắc chắn không có secret/build artifact/untracked file thừa.

## 9. Những gì không còn là critical path lab CD

- Không cần ArgoCD sync để deploy lab.
- Không cần commit bump image tag vào GitOps app để lab nhận image mới.
- Không cần Consul CRD trong lab profile vì `values-lab-4gb.yaml` tắt `consul.serviceDefaults.enabled`.
- Không cần Traefik ingress controller trong K3s lab; chart lab dùng ingress-nginx.
