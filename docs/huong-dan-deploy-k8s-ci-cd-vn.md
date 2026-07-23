# Hướng dẫn deploy NexusChat bằng K8s và CI/CD

Tài liệu này trả lời đúng 2 câu hỏi:

1. Có thể “dồn” 6 image hiện tại vào một image duy nhất không.
2. Nếu đang chạy NexusChat trên K8s, quy trình `git pull` / `git commit` / `git push` có tự deploy lại qua workflow hay không.

## Kết luận ngắn

Không nên gộp 6 service thành một image duy nhất nếu mục tiêu là vận hành ổn định, rollback dễ, và giữ đúng ranh giới microservice.

Điều nên hiểu đúng là:

- Có thể gom các image vào cùng một Docker Hub namespace hoặc cùng một repository logic.
- Nhưng “một repository Docker Hub” và “một image duy nhất” là hai thứ khác nhau.
- Với NexusChat hiện tại, repo đang được thiết kế để build và deploy nhiều image riêng:
  - `tuananh165/nexuschat-web`
  - `tuananh165/nexuschat-api`
  - `tuananh165/nexuschat-ai-service`
  - `tuananh165/nexuschat-presence`
  - `tuananh165/nexuschat-notification`
  - `tuananh165/nexuschat-call`

## Có thể gộp không

### 1. Gộp thành một repository Docker Hub

Có.

Ví dụ bạn có thể chuẩn hóa mọi image dưới cùng namespace `tuananh165`, nhưng vẫn là nhiều image khác tag:

- `docker.io/tuananh165/nexuschat:api-<tag>`
- `docker.io/tuananh165/nexuschat:web-<tag>`
- `docker.io/tuananh165/nexuschat:ai-service-<tag>`
- `docker.io/tuananh165/nexuschat:presence-<tag>`
- `docker.io/tuananh165/nexuschat:notification-<tag>`
- `docker.io/tuananh165/nexuschat:call-<tag>`

Nhưng cách này chỉ là đổi cách đặt tên. Nó không biến 6 service thành 1 runtime duy nhất.

### 2. Gộp thành đúng 1 image digest dùng chung cho cả 6 service

Về mặt kỹ thuật là có thể, nhưng không khuyến nghị.

Muốn làm vậy, bạn phải:

- tạo một image rất lớn chứa Go binary, frontend static files, Python runtime cho `ai-service`,
- thêm entrypoint/command khác nhau để chạy từng service,
- chỉnh Helm để các Deployment cùng trỏ về một image duy nhất nhưng khác `command`/`args`,
- chỉnh CI/CD để build và push một image “đa năng” thay vì nhiều image chuyên biệt.

Nhược điểm:

- image phình lớn,
- build chậm hơn,
- rollback kém rõ ràng hơn,
- thay đổi nhỏ ở web có thể kéo theo rebuild cả image dùng cho backend và AI,
- khó giữ ranh giới kỹ thuật giữa Go, Node/Next.js, Python/FastAPI.

Với NexusChat hiện tại, phương án đúng hơn là giữ image tách theo service.

## Workflow hiện tại của repo đang làm gì

File workflow chính là:

`[.github/workflows/devsecops-platform.yml](../.github/workflows/devsecops-platform.yml)`

Nó đang làm theo luồng này:

1. Pull request: chạy kiểm tra, test, lint, scan.
2. Push vào `main` hoặc `kafka`, hoặc tag `v*`: test, build image, scan image, tạo SBOM, ký image.
3. Push vào `main`: ngoài các bước trên còn có job deploy K8s bằng Helm.

### Các image hiện tại được build/push

- `nexuschat-api`
- `nexuschat-web`
- `nexuschat-ai-service`
- `nexuschat-presence`
- `nexuschat-notification`
- `nexuschat-call`

### Các image proxy variant

Workflow hiện tại còn build thêm các tag proxy cho web/api:

- `nexuschat-api:proxy-upload`
- `nexuschat-web:proxy-upload`
- `nexuschat-web:proxy-upload-v2`

## Câu trả lời đúng cho câu hỏi “server chỉ cần git pull và git commit, push là tự deploy không”

### Nếu server của bạn là self-hosted runner K8s

Đúng, nhưng cần hiểu chính xác:

- Không phải `git pull` trên server sẽ tự deploy.
- Workflow chỉ chạy khi bạn `git push` lên GitHub ở nhánh/tag mà workflow đã khai báo.
- Khi push xong:
  - GitHub Actions chạy CI,
  - build/push image lên Docker Hub,
  - nếu là push vào `main` thì job `deploy-lab-k8s` sẽ chạy trên self-hosted runner,
  - job này dùng `helm upgrade --install` để rollout lại K8s.

### Nếu server chỉ là máy chạy K8s, không phải runner

Thì không đủ.

Bạn cần:

- một GitHub Actions self-hosted runner có label `self-hosted, linux, x64, k3s-lab`,
- kubeconfig hợp lệ hoặc `KUBE_CONFIG_B64`,
- secrets Docker Hub và runtime secret đã được cấu hình.

## Quy trình deploy lại đúng cách

### Cách 1: deploy tự động qua workflow

1. Sửa code.
2. `git add`, `git commit`.
3. `git push` lên nhánh `main` nếu muốn deploy production/lab theo workflow hiện tại.
4. Chờ workflow chạy xong.
5. Kiểm tra rollout trên K8s.

### Cách 2: deploy thủ công bằng Helm

Khi bạn muốn tự kiểm soát version:

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-k3s.yaml \
  --set-string imageDefaults.tag="<GIT_SHA_OR_TAG>" \
  --set-string services.web.image.fullname="docker.io/tuananh165/nexuschat-web:proxy-upload-v2-<GIT_SHA_OR_TAG>" \
  --set-string services.uploader.image.fullname="docker.io/tuananh165/nexuschat-api:proxy-upload-<GIT_SHA_OR_TAG>" \
  --wait \
  --timeout 10m
```

## Checklist trước khi bấm deploy

- Docker Hub secrets đã có:
  - `DOCKER_USERNAME`
  - `DOCKER_PASSWORD`
- Nếu deploy bằng runner riêng:
  - `KUBE_CONFIG_B64` hợp lệ, hoặc runner đã có kubeconfig sẵn.
- Runtime secret trong namespace K8s đã được tạo:
  - JWT secret
  - Redis password
  - Cassandra credentials
  - OAuth Google credentials
  - AI service credentials
  - VAPID secret
  - TURN shared secret
- DNS và ingress đã trỏ đúng domain:
  - `nexuschat.click`
  - `turn.nexuschat.click`

## Các bước deploy lại trên server K8s

### Bước 1: kiểm tra runner và cluster

```bash
kubectl config current-context
kubectl get nodes
```

Nếu `kubectl get nodes` lỗi, thì workflow deploy không thể hoàn tất cho tới khi cluster/API server hoạt động lại.

### Bước 2: push code lên GitHub

```bash
git add .
git commit -m "feat: update nexuschat deploy"
git push origin main
```

Nếu branch không phải `main`, workflow hiện tại có thể chỉ chạy CI chứ không deploy.

### Bước 3: theo dõi workflow

Trong GitHub Actions:

- job `test`
- job `helm`
- job `secret-scan`
- job `codeql`
- job `trivy-fs`
- job `build-images`
- job `build-proxy-variants`
- job `deploy-lab-k8s`

### Bước 4: kiểm tra rollout

```bash
kubectl -n nexuschat-lab get deploy
kubectl -n nexuschat-lab rollout status deployment/web --timeout=600s
kubectl -n nexuschat-lab rollout status deployment/chat --timeout=600s
kubectl -n nexuschat-lab rollout status deployment/match --timeout=600s
kubectl -n nexuschat-lab rollout status deployment/user --timeout=600s
kubectl -n nexuschat-lab rollout status deployment/uploader --timeout=600s
kubectl -n nexuschat-lab rollout status deployment/forwarder --timeout=600s
kubectl -n nexuschat-lab rollout status deployment/ai-service --timeout=600s
kubectl -n nexuschat-lab rollout status deployment/presence --timeout=600s
kubectl -n nexuschat-lab rollout status deployment/notification --timeout=600s
kubectl -n nexuschat-lab rollout status deployment/call --timeout=600s
```

## Những file đáng chú ý trong repo hiện tại

- Workflow CI/CD: `[.github/workflows/devsecops-platform.yml](../.github/workflows/devsecops-platform.yml)`
- Helm chart: `[deployments/helm/nexuschat](../deployments/helm/nexuschat)`
- Lab values: `[deployments/helm/nexuschat/values-lab-k3s.yaml](../deployments/helm/nexuschat/values-lab-k3s.yaml)`
- Docker Compose: `[docker-compose.yaml](../docker-compose.yaml)`
- Runtime guide: `[docs/devsecops-implementation-runbook.md](devsecops-implementation-runbook.md)`
- K8s deploy guide: `[docs/deploy-k8s-guide.md](deploy-k8s-guide.md)`
- Docker Hub direct rollout guide: `[docs/dockerhub-direct-k8s-rollout.md](dockerhub-direct-k8s-rollout.md)`

## Khuyến nghị thực tế

Nếu mục tiêu của bạn là vận hành ổn định trên K8s, hãy giữ mô hình hiện tại:

- 1 image cho web,
- 1 image cho Go backend core,
- 1 image cho AI service,
- 3 image cho presence/notification/call.

Đó là cách cân bằng tốt nhất giữa:

- tốc độ deploy,
- rollback,
- quan sát lỗi,
- tách trách nhiệm,
- và bảo trì lâu dài.
