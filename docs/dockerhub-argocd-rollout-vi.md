# Hướng dẫn chuyển image NexusChat sang Docker Hub và rollout qua GitHub Actions + ArgoCD

Tài liệu này hướng dẫn đầy đủ quy trình:

1. Tạo repository trên Docker Hub
2. Thiết lập GitHub Actions secrets
3. Rà soát thay đổi và commit đúng phạm vi
4. Push commit lên GitHub
5. Chạy workflow `devsecops-platform.yml`
6. Xác nhận image đã được push lên Docker Hub
7. Để ArgoCD staging tự sync và kiểm tra kết quả
8. Kiểm thử staging
9. Cập nhật tag production và sync production
10. Checklist rollback khi cần

Tài liệu này phản ánh trạng thái repo hiện tại, trong đó image đã được chuyển sang Docker Hub với namespace sau:

- `docker.io/tuananh165/nexuschat-api`
- `docker.io/tuananh165/nexuschat-web`
- `docker.io/tuananh165/nexuschat-ai-service`

Ngoài ra workflow cũng đã được mở rộng để build các biến thể phục vụ lab/proxy:

- `docker.io/tuananh165/nexuschat-api:proxy-upload`
- `docker.io/tuananh165/nexuschat-web:proxy-upload`
- `docker.io/tuananh165/nexuschat-web:proxy-upload-v2`

## 0. Thông tin quan trọng cần biết trước

### 0.1. Các file quan trọng trong repo

- Workflow CI/CD chính: `.github/workflows/devsecops-platform.yml`
- ArgoCD staging: `deployments/gitops/applications/nexuschat-staging.yaml`
- ArgoCD production: `deployments/gitops/applications/nexuschat-production.yaml`
- Helm values mặc định: `deployments/helm/nexuschat/values.yaml`
- Helm values lab: `deployments/helm/nexuschat/values-lab-4gb.yaml`

### 0.2. Cơ chế deploy hiện tại

Staging:
- Theo dõi branch `main`
- ArgoCD có `syncPolicy.automated`
- Khi manifest trên `main` thay đổi và image tương ứng tồn tại trên registry, staging sẽ tự đồng bộ

Production:
- Cũng đọc manifest từ repo Git
- Nhưng image đang được pin bằng `imageDefaults.tag` trong `deployments/gitops/applications/nexuschat-production.yaml`
- Không có `automated` sync
- Cần cập nhật tag rồi sync thủ công

### 0.3. Quy ước tag khuyến nghị

Nên dùng tag bất biến:
- SHA commit: ví dụ `90d41a588e4d2fb2876e9afaab759717dfa9e801`
- Hoặc release tag: ví dụ `v0.1.0`

Không nên dùng `latest` cho production.

## 1. Tạo Docker Hub repositories

Đăng nhập Docker Hub bằng tài khoản `tuananh165`.

Tạo 3 repository chính sau:

- `nexuschat-api`
- `nexuschat-web`
- `nexuschat-ai-service`

Khuyến nghị:
- Visibility: Private nếu cluster có pull secret và bạn muốn kiểm soát truy cập
- Visibility: Public nếu muốn staging/lab pull đơn giản hơn

Nếu bạn đang dùng image lab/proxy, không cần tạo repo riêng cho chúng vì chúng dùng chung repo với image chính, chỉ khác tag.

Ví dụ:
- `tuananh165/nexuschat-api:proxy-upload`
- `tuananh165/nexuschat-web:proxy-upload-v2`

## 2. Thiết lập GitHub Actions secrets

Vào GitHub repository:
- `Settings` → `Secrets and variables` → `Actions`

Tạo 2 secret sau:

- `DOCKER_USERNAME`
  - giá trị: `tuananh165`
- `DOCKER_PASSWORD`
  - giá trị: Docker Hub access token

Khuyến nghị rất mạnh:
- Không dùng password tài khoản Docker Hub thật
- Hãy tạo Access Token trong Docker Hub rồi dùng token đó làm `DOCKER_PASSWORD`

### 2.1. Cách tạo Docker Hub Access Token

Trên Docker Hub:
- `Account Settings` → `Personal access tokens`
- Tạo token mới, ví dụ tên: `github-actions-nexuschat`
- Copy token ngay lúc tạo
- Dán vào GitHub secret `DOCKER_PASSWORD`

## 3. Rà soát thay đổi trước khi commit

Trước khi commit, bắt buộc kiểm tra lại diff để tránh đẩy nhầm các file không liên quan.

### 3.1. Xem toàn bộ trạng thái

```bash
git status --short
```

### 3.2. Xem diff các file đang thay đổi

```bash
git diff --name-status
```

### 3.3. Nhóm file nên commit cho đợt đổi registry này

Nên commit:
- `.github/workflows/devsecops-platform.yml`
- `.github/workflows/docker-api-dev.yml`
- `.github/workflows/docker-api-release.yaml`
- `.github/workflows/docker-web-dev.yaml`
- `.github/workflows/docker-web-release.yaml`
- `deployments/helm/nexuschat/values.yaml`
- `deployments/helm/nexuschat/values-lab-4gb.yaml`
- `build/Dockerfile.web-patch`
- `build/Dockerfile.web-update`
- `Makefile`
- `docker-compose.yaml`
- `README.md`
- Tài liệu này: `docs/dockerhub-argocd-rollout-vi.md`

Nếu workflow proxy variants là một phần scope chính thức của đợt này thì cũng nên commit:
- phần thay đổi thêm job `build-proxy-variants` trong `.github/workflows/devsecops-platform.yml`

### 3.4. File generated cần rà kỹ

Trong môi trường local hiện tại, `make build` đã regenerate các file Swagger docs tracked trong repo:

- `docs/chat/chat_docs.go`
- `docs/chat/chat_swagger.json`
- `docs/chat/chat_swagger.yaml`
- `docs/match/match_docs.go`
- `docs/match/match_swagger.json`
- `docs/match/match_swagger.yaml`
- `docs/uploader/uploader_docs.go`
- `docs/uploader/uploader_swagger.json`
- `docs/uploader/uploader_swagger.yaml`
- `docs/user/user_docs.go`
- `docs/user/user_swagger.json`
- `docs/user/user_swagger.yaml`

Khuyến nghị cho đợt đổi registry:
- Không commit các file generated Swagger ở trên nếu bạn không chủ đích cập nhật API docs
- Lý do: thay đổi registry Docker không làm thay đổi API contract
- Các file này có thể thay đổi mạnh do khác version của `swag`/Go toolchain, dễ làm diff bị nhiễu

Nếu muốn loại chúng khỏi commit:

```bash
git restore \
  docs/chat/chat_docs.go \
  docs/chat/chat_swagger.json \
  docs/chat/chat_swagger.yaml \
  docs/match/match_docs.go \
  docs/match/match_swagger.json \
  docs/match/match_swagger.yaml \
  docs/uploader/uploader_docs.go \
  docs/uploader/uploader_swagger.json \
  docs/uploader/uploader_swagger.yaml \
  docs/user/user_docs.go \
  docs/user/user_swagger.json \
  docs/user/user_swagger.yaml
```

### 3.5. File `internal/wire/wire_gen.go`

`make build` có chạy `wire`, nhưng nếu file này không đổi trong `git diff` thì không cần làm gì thêm.

Kiểm tra nhanh:

```bash
git diff -- internal/wire/wire_gen.go
```

Nếu không có output thì không có thay đổi cần commit.

### 3.6. File khác không thuộc scope đợt này

Nếu `git status` còn hiển thị nhiều file unrelated từ các task khác trước đó, tuyệt đối không commit chung. Chỉ stage các file đúng scope.

## 4. Commit và push lên GitHub

### 4.1. Stage đúng file

Ví dụ:

```bash
git add \
  .github/workflows/devsecops-platform.yml \
  .github/workflows/docker-api-dev.yml \
  .github/workflows/docker-api-release.yaml \
  .github/workflows/docker-web-dev.yaml \
  .github/workflows/docker-web-release.yaml \
  deployments/helm/nexuschat/values.yaml \
  deployments/helm/nexuschat/values-lab-4gb.yaml \
  build/Dockerfile.web-patch \
  build/Dockerfile.web-update \
  Makefile \
  docker-compose.yaml \
  README.md \
  docs/dockerhub-argocd-rollout-vi.md
```

Nếu bạn cũng muốn commit các file mới khác nằm trong scope thực sự của rollout thì thêm rõ ràng từng file một, không dùng `git add .`.

### 4.2. Kiểm tra lại những gì sắp commit

```bash
git diff --cached --name-status
```

### 4.3. Commit

```bash
git commit -m "Switch container images from GHCR to Docker Hub"
```

### 4.4. Push

Nếu đang làm trên `main`:

```bash
git push origin main
```

Nếu đang làm trên branch riêng:

```bash
git push origin <ten-branch>
```

Sau đó merge vào `main` theo quy trình làm việc của bạn.

## 5. Chạy workflow `devsecops-platform.yml`

Workflow chính hiện chạy khi:
- push lên `main`
- push lên `kafka`
- push tag `v*`
- chạy tay bằng `workflow_dispatch`

### 5.1. Cách chạy khuyến nghị

Sau khi push commit lên `main`, vào GitHub Actions và kiểm tra workflow:
- `DevSecOps Platform Pipeline`

Hoặc chạy tay bằng `workflow_dispatch` nếu cần rerun.

### 5.2. Kết quả mong đợi

Workflow phải:
1. Test code
2. Validate Helm
3. Build 3 image chính
4. Push image chính lên Docker Hub
5. Build/push proxy variants
6. Scan image
7. Tạo SBOM
8. Ký image bằng Cosign

## 6. Xác nhận image đã có trên Docker Hub

Sau khi workflow thành công, kiểm tra trên Docker Hub UI hoặc dùng lệnh.

### 6.1. Kiểm tra bằng trình duyệt

Mở từng repo trên Docker Hub và xác nhận có các tag mới:

- `tuananh165/nexuschat-api`
- `tuananh165/nexuschat-web`
- `tuananh165/nexuschat-ai-service`

Tag cần thấy:
- tag SHA commit hiện tại, ví dụ `90d41a588e4d2fb2876e9afaab759717dfa9e801`
- hoặc release tag `vX.Y.Z`
- `sha-*` nếu workflow metadata tạo thêm

Proxy variants cần thấy thêm:
- `proxy-upload`
- `proxy-upload-<sha-or-tag>`
- `proxy-upload-v2`
- `proxy-upload-v2-<sha-or-tag>`

### 6.2. Kiểm tra bằng Docker CLI

```bash
docker pull docker.io/tuananh165/nexuschat-api:<TAG>
docker pull docker.io/tuananh165/nexuschat-web:<TAG>
docker pull docker.io/tuananh165/nexuschat-ai-service:<TAG>
```

Nếu staging/lab dùng proxy variants thì kiểm tra thêm:

```bash
docker pull docker.io/tuananh165/nexuschat-api:proxy-upload
docker pull docker.io/tuananh165/nexuschat-web:proxy-upload
docker pull docker.io/tuananh165/nexuschat-web:proxy-upload-v2
```

## 7. Để ArgoCD staging tự sync

Staging app hiện theo `main` và có automated sync.

File xác nhận:
- `deployments/gitops/applications/nexuschat-staging.yaml`

Các điểm quan trọng:
- `targetRevision: main`
- `syncPolicy.automated.prune: true`
- `syncPolicy.automated.selfHeal: true`

### 7.1. Điều kiện để staging rollout thành công

Cả 2 điều kiện sau phải đúng:
1. Commit manifest mới đã lên branch `main`
2. Image mà manifest tham chiếu đã tồn tại trên Docker Hub

Nếu một trong hai chưa đúng, staging có thể sync lỗi hoặc pod pull image thất bại.

### 7.2. Kiểm tra trạng thái ArgoCD staging

Nếu có ArgoCD CLI:

```bash
argocd app get nexuschat-staging
argocd app wait nexuschat-staging --health --sync
```

Nếu chỉ có kubectl:

```bash
kubectl -n argocd get application nexuschat-staging -o yaml
kubectl -n nexuschat-staging get deploy,pod,svc,ingress
kubectl -n nexuschat-staging rollout status deploy/nexuschat-web
kubectl -n nexuschat-staging rollout status deploy/nexuschat-chat
kubectl -n nexuschat-staging rollout status deploy/nexuschat-user
kubectl -n nexuschat-staging rollout status deploy/nexuschat-match
kubectl -n nexuschat-staging rollout status deploy/nexuschat-uploader
kubectl -n nexuschat-staging rollout status deploy/nexuschat-forwarder
kubectl -n nexuschat-staging rollout status deploy/nexuschat-ai-service
```

### 7.3. Nếu staging không tự sync

Thử sync tay:

```bash
argocd app sync nexuschat-staging
argocd app wait nexuschat-staging --health --sync
```

Hoặc:

```bash
kubectl -n argocd annotate application nexuschat-staging rollout-trigger=$(date +%s) --overwrite
```

## 8. Test staging

Sau khi staging sync xong, kiểm tra theo checklist sau.

### 8.1. Kiểm tra pod và image thực tế

```bash
kubectl -n nexuschat-staging get pods -o wide
kubectl -n nexuschat-staging get deploy -o jsonpath='{range .items[*]}{.metadata.name}{" => "}{range .spec.template.spec.containers[*]}{.image}{" "}{end}{"\n"}{end}'
```

Xác nhận image đang chạy là Docker Hub image mới, ví dụ:
- `docker.io/tuananh165/nexuschat-api:<TAG>`
- `docker.io/tuananh165/nexuschat-web:<TAG>`
- `docker.io/tuananh165/nexuschat-ai-service:<TAG>`

### 8.2. Kiểm tra ingress/service

```bash
kubectl -n nexuschat-staging get ingress
kubectl -n nexuschat-staging get svc
```

### 8.3. Kiểm tra log nếu pod lỗi

```bash
kubectl -n nexuschat-staging logs deploy/nexuschat-web --tail=200
kubectl -n nexuschat-staging logs deploy/nexuschat-chat --tail=200
kubectl -n nexuschat-staging logs deploy/nexuschat-user --tail=200
kubectl -n nexuschat-staging logs deploy/nexuschat-uploader --tail=200
kubectl -n nexuschat-staging logs deploy/nexuschat-ai-service --tail=200
```

### 8.4. Smoke test tối thiểu

Kiểm tra:
- web mở được
- đăng nhập hoạt động
- chat API còn sống
- uploader hoạt động
- AI service còn trả lời

Ví dụ kiểm tra HTTP cơ bản:

```bash
curl -I https://staging.nexuschat.example.com/
curl -I https://staging.nexuschat.example.com/api/user/swagger/index.html
curl -I https://staging.nexuschat.example.com/api/chat/swagger/index.html
curl -I https://staging.nexuschat.example.com/api/uploader/swagger/index.html
```

Nếu staging domain thực tế khác, thay lại đúng domain hiện dùng.

## 9. Cập nhật tag production rồi sync production

Production hiện đang pin tag ở file:
- `deployments/gitops/applications/nexuschat-production.yaml`

Hiện tại trường quan trọng là:
- `imageDefaults.tag`

### 9.1. Chọn tag sẽ promote

Khuyến nghị dùng đúng tag đã chạy tốt ở staging, ví dụ:
- `90d41a588e4d2fb2876e9afaab759717dfa9e801`
- hoặc `v0.1.0`

### 9.2. Sửa tag production

Ví dụ đổi thành tag mới:

```yaml
- name: imageDefaults.tag
  value: 90d41a588e4d2fb2876e9afaab759717dfa9e801
```

### 9.3. Commit thay đổi production

```bash
git add deployments/gitops/applications/nexuschat-production.yaml
git commit -m "Promote production images to <TAG>"
git push origin main
```

### 9.4. Sync production

Nếu có ArgoCD CLI:

```bash
argocd app sync nexuschat-production
argocd app wait nexuschat-production --health --sync
```

Nếu dùng kubectl để theo dõi:

```bash
kubectl -n argocd get application nexuschat-production -o yaml
kubectl -n nexuschat-prod get deploy,pod,svc,ingress
kubectl -n nexuschat-prod rollout status deploy/nexuschat-web
kubectl -n nexuschat-prod rollout status deploy/nexuschat-chat
kubectl -n nexuschat-prod rollout status deploy/nexuschat-user
kubectl -n nexuschat-prod rollout status deploy/nexuschat-match
kubectl -n nexuschat-prod rollout status deploy/nexuschat-uploader
kubectl -n nexuschat-prod rollout status deploy/nexuschat-forwarder
kubectl -n nexuschat-prod rollout status deploy/nexuschat-ai-service
```

## 10. Checklist rollout staging/prod

## 10.1. Checklist staging

- [ ] Đã tạo đủ 3 Docker Hub repos
- [ ] Đã set `DOCKER_USERNAME`
- [ ] Đã set `DOCKER_PASSWORD`
- [ ] Đã rà lại `git diff` và loại bỏ file generated không cần commit
- [ ] Đã push manifest mới lên `main`
- [ ] Workflow `devsecops-platform.yml` chạy thành công
- [ ] Docker Hub đã có image tag mới cho `api`, `web`, `ai-service`
- [ ] Docker Hub đã có proxy tags nếu staging/lab cần dùng
- [ ] ArgoCD staging đã sync thành công
- [ ] Tất cả deployment ở namespace `nexuschat-staging` đã rollout thành công
- [ ] Smoke test staging pass

## 10.2. Checklist production

- [ ] Staging đã chạy ổn với đúng tag muốn promote
- [ ] Docker Hub đã có đủ image của tag đó
- [ ] Đã cập nhật `imageDefaults.tag` trong `deployments/gitops/applications/nexuschat-production.yaml`
- [ ] Đã commit và push thay đổi production
- [ ] Đã sync `nexuschat-production`
- [ ] Tất cả deployment ở namespace `nexuschat-prod` đã rollout thành công
- [ ] Kiểm tra log không có lỗi pull image hoặc crash loop
- [ ] Smoke test production pass

## 11. Checklist rollback production

Nếu production có sự cố sau rollout:

1. Xác định tag ổn định gần nhất
2. Sửa lại `imageDefaults.tag` trong `deployments/gitops/applications/nexuschat-production.yaml`
3. Commit + push lên `main`
4. Sync lại ArgoCD production
5. Theo dõi rollout và log

Ví dụ:

```bash
argocd app sync nexuschat-production
argocd app wait nexuschat-production --health --sync
kubectl -n nexuschat-prod get pods
```

## 12. Lệnh tham khảo: build/push thủ công nếu CI gặp sự cố

Đăng nhập:

```bash
docker login -u tuananh165
```

Đặt tag:

```bash
export TAG=$(git rev-parse HEAD)
```

Build/push image chính:

```bash
docker build -f build/Dockerfile.api \
  --build-arg VERSION=$TAG \
  -t docker.io/tuananh165/nexuschat-api:$TAG \
  .

docker push docker.io/tuananh165/nexuschat-api:$TAG

docker build -f build/Dockerfile.web \
  --build-arg VERSION=$TAG \
  -t docker.io/tuananh165/nexuschat-web:$TAG \
  .

docker push docker.io/tuananh165/nexuschat-web:$TAG

docker build -f ai-service/Dockerfile \
  -t docker.io/tuananh165/nexuschat-ai-service:$TAG \
  ai-service

docker push docker.io/tuananh165/nexuschat-ai-service:$TAG
```

Build/push proxy variants:

```bash
docker build -f build/Dockerfile.api \
  --build-arg VERSION=$TAG \
  -t docker.io/tuananh165/nexuschat-api:proxy-upload \
  -t docker.io/tuananh165/nexuschat-api:proxy-upload-$TAG \
  .

docker push docker.io/tuananh165/nexuschat-api:proxy-upload
docker push docker.io/tuananh165/nexuschat-api:proxy-upload-$TAG

docker build -f build/Dockerfile.web \
  --build-arg VERSION=$TAG \
  -t docker.io/tuananh165/nexuschat-web:proxy-upload \
  -t docker.io/tuananh165/nexuschat-web:proxy-upload-$TAG \
  .

docker push docker.io/tuananh165/nexuschat-web:proxy-upload
docker push docker.io/tuananh165/nexuschat-web:proxy-upload-$TAG

docker build -f build/Dockerfile.web-patch \
  -t docker.io/tuananh165/nexuschat-web:proxy-upload-v2 \
  -t docker.io/tuananh165/nexuschat-web:proxy-upload-v2-$TAG \
  .

docker push docker.io/tuananh165/nexuschat-web:proxy-upload-v2
docker push docker.io/tuananh165/nexuschat-web:proxy-upload-v2-$TAG
```

## 13. Kết luận

Quy trình an toàn nhất là:

1. Dọn diff, chỉ commit đúng file liên quan
2. Push lên `main`
3. Để GitHub Actions build/push image lên Docker Hub
4. Xác nhận image tồn tại
5. Để staging auto sync và test kỹ
6. Chỉ sau khi staging ổn mới cập nhật `imageDefaults.tag` cho production
7. Sync production thủ công và theo dõi rollout
