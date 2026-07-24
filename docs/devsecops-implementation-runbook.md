# DevSecOps runbook cho Safety, Discovery và Workspace

## Phạm vi

Workflow `.github/workflows/devsecops-platform.yml` build, scan, ký, push và deploy các image `api`, `web`, `ai-service`, `safety`, `discovery`, `workspace`. Presence, Notification, Call/WebRTC, VAPID, TURN và TLS không còn trong release.

## Local validation

```powershell
go test ./...
go test -race ./pkg/realtime ./pkg/safety ./pkg/discovery ./pkg/workspace ./pkg/chat ./pkg/match
cd frontend
npm.cmd ci
npm.cmd run build
cd ..
docker compose config
docker compose build safety discovery workspace
```

## Cassandra migration

Backup keyspace trước khi chạy:

```bash
kubectl -n cassandra exec deploy/cassandra -- cqlsh -e "DESCRIBE KEYSPACE NexusChat"
kubectl -n cassandra cp deploy/cassandra:/var/lib/cassandra/data ./backup-cassandra
kubectl -n cassandra exec deploy/cassandra -- cqlsh -f /migrations/002_replace_realtime_services.cql
```

Fresh cluster dùng `cassandra/init.cql`. Cluster đang chạy dùng `cassandra/migrations/002_replace_realtime_services.cql`; migration idempotent ở các lệnh `DROP TABLE IF EXISTS`.

## Docker Hub

Tag immutable bằng Git SHA:

```bash
docker login
docker build --build-arg SERVICE=safety -t tuananh165/nexuschat-safety:$GIT_SHA -f build/Dockerfile.realtime .
docker build --build-arg SERVICE=discovery -t tuananh165/nexuschat-discovery:$GIT_SHA -f build/Dockerfile.realtime .
docker build --build-arg SERVICE=workspace -t tuananh165/nexuschat-workspace:$GIT_SHA -f build/Dockerfile.realtime .
docker push tuananh165/nexuschat-safety:$GIT_SHA
docker push tuananh165/nexuschat-discovery:$GIT_SHA
docker push tuananh165/nexuschat-workspace:$GIT_SHA
```

CI thực hiện cùng các bước này, cộng Trivy, SPDX SBOM và Cosign.

## K3s HTTP-only

```bash
kubectl create namespace nexuschat-lab --dry-run=client -o yaml | kubectl apply -f -
helm upgrade --install nexuschat deployments/helm/nexuschat \
  -n nexuschat-lab --create-namespace \
  -f deployments/helm/nexuschat/values.yaml \
  -f deployments/helm/nexuschat/values-lab-k3s.yaml \
  --set-string imageDefaults.tag="$GIT_SHA" \
  --atomic --wait --timeout 10m
```

Chart không render `spec.tls`, không dùng cert-manager và không redirect HTTPS. Host lab là `nexuschat.click`; WebSocket dùng `ws://`.

Kiểm tra:

```bash
kubectl -n nexuschat-lab get deploy,svc,ingress
for name in safety discovery workspace; do
  kubectl -n nexuschat-lab rollout status deploy/$name --timeout=600s
  kubectl -n nexuschat-lab run smoke-$name --rm -i --restart=Never \
    --image=curlimages/curl:8.12.1 -- curl --fail http://$name:$(case $name in safety) echo 5005;; discovery) echo 5006;; workspace) echo 5007;; esac)/ready
done
```

## Rollback

```bash
helm history nexuschat -n nexuschat-lab
helm rollback nexuschat <REVISION> -n nexuschat-lab --atomic --wait
```

Rollback source code không khôi phục bảng đã drop; phải phục hồi từ backup Cassandra nếu cần dữ liệu cũ.
