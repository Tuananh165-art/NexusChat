# NexusChat

NexusChat là nền tảng chat realtime Go/Next.js dành cho matching ẩn danh. Runtime hiện tại chạy HTTP và WebSocket (`ws://`), không dùng Web Push, WebRTC, TURN, TLS Secret hoặc cert-manager.

## Kiến trúc hiện tại

```text
Browser
  │ HTTP + WebSocket
  ▼
Ingress/Traefik ── web ── chat ── match
                         │      │
                         │      ├── discovery (matching theo sở thích)
                         │      └── safety (block/risk filter)
                         ├── safety (moderation trước broadcast)
                         └── workspace (task/note/bookmark/reminder)

Các Go service dùng Kafka + Redis + Cassandra + gRPC.
AI chạy riêng bằng FastAPI và chỉ được gọi khi cấu hình optional.
```

## Service

| Service | Command | Image | Chức năng |
|---|---|---|---|
| web | `server web` | `tuananh165/nexuschat-web` | Static Next.js |
| api | `server chat`, `match`, `user`, `uploader`, `forwarder` | `tuananh165/nexuschat-api` | Chat, matching, user, upload và fanout |
| ai-service | `uvicorn app.main:app` | `tuananh165/nexuschat-ai-service` | Rewrite và workflow AI |
| safety | `server safety` | `tuananh165/nexuschat-safety` | Moderation, report, block, risk score |
| discovery | `server discovery` | `tuananh165/nexuschat-discovery` | Profile sở thích, ranking candidate, feedback |
| workspace | `server workspace` | `tuananh165/nexuschat-workspace` | Task, note, bookmark, Kanban, reminder |

Presence, Notification, Call/WebRTC, Web Push và Coturn đã bị loại bỏ hoàn toàn.

## API chính

Safety:

- `POST/GET /api/safety/reports`
- `PUT /api/safety/reports/{id}/status`
- `POST /api/safety/reports/{id}/appeals`
- `GET/POST /api/safety/blocks`, `DELETE /api/safety/blocks/{userId}`
- `GET/POST/PUT/DELETE /api/safety/rules`
- `GET /api/safety/decisions`

Discovery:

- `GET/PUT/DELETE /api/discovery/profile`
- `GET/PUT /api/discovery/interests`
- `GET /api/discovery/recommendations`
- `GET /api/discovery/match-history`
- `POST/PUT/DELETE /api/discovery/feedback`

Workspace:

- `GET/POST /api/workspace/items`
- `GET/PUT/DELETE /api/workspace/items/{id}`
- `PUT /api/workspace/items/{id}/status`
- `PUT /api/workspace/items/{id}/assignees`
- `GET /api/workspace/boards/{channelId}`
- `GET /api/workspace/bookmarks`
- `POST/PUT/DELETE /api/workspace/collections`
- `GET /api/workspace/reminders/due`
- `GET /api/workspace/ws`

## Chạy local

Yêu cầu Go 1.24, Node.js 20+, Docker Desktop và các dependency Kafka, Redis, Cassandra.

```powershell
Copy-Item .env.example .env
docker compose config
docker compose up -d cassandra kafka redis-node-5 cassandra-init
docker compose up -d
```

Chạy backend:

```powershell
go run . chat
go run . match
go run . safety
go run . discovery
go run . workspace
```

Chạy frontend:

```powershell
cd frontend
npm.cmd ci
npm.cmd run dev
```

## Event và concurrency

Các topic Kafka version hóa:

- `nexuschat.chat.events.v1`
- `nexuschat.safety.events.v1`
- `nexuschat.discovery.events.v1`
- `nexuschat.workspace.events.v1`

Consumer dùng envelope chung, idempotency bằng `processed_events`, outbox relay, retry backoff và DLQ. Worker pool dùng bounded queue, `context`, `errgroup` và graceful shutdown để tránh goroutine leak.

## Database

`cassandra/init.cql` chứa schema cho Safety, Discovery, Workspace. Migration `cassandra/migrations/002_replace_realtime_services.cql` xóa bảng notification/call cũ sau khi backup. Redis chỉ dùng cho cache, rate limit, lock, waitlist, reminder lease và dedup nhanh.

## Docker image

```powershell
docker compose build safety discovery workspace
docker build --build-arg SERVICE=safety -f build/Dockerfile.realtime -t tuananh165/nexuschat-safety:dev .
docker build --build-arg SERVICE=discovery -f build/Dockerfile.realtime -t tuananh165/nexuschat-discovery:dev .
docker build --build-arg SERVICE=workspace -f build/Dockerfile.realtime -t tuananh165/nexuschat-workspace:dev .
```

CI/CD dùng Git SHA immutable tag, Trivy, SBOM, Cosign và push ba image mới lên Docker Hub.

## Kubernetes HTTP-only

Profile lab: `deployments/helm/nexuschat/values-lab-k3s.yaml`.

Profile đã cấu hình:

- ingress-nginx, host `nexuschat.click`
- `tlsSecretName: ""`
- `ssl-redirect: "false"`
- paths `/api/safety`, `/api/discovery`, `/api/workspace`
- Deployment `safety`, `discovery`, `workspace`

Deploy:

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --create-namespace \
  -f deployments/helm/nexuschat/values.yaml \
  -f deployments/helm/nexuschat/values-lab-k3s.yaml \
  --set-string imageDefaults.tag="$GIT_SHA" \
  --atomic --wait --timeout 10m
```

Workflow `.github/workflows/devsecops-platform.yml` tự chạy khi push branch cấu hình, build/test/scan/push và rollout mười Deployment: `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, `ai-service`, `safety`, `discovery`, `workspace`.

## Kiểm tra trước khi commit

```powershell
go test ./...
go test -race ./pkg/realtime ./pkg/safety ./pkg/discovery ./pkg/workspace ./pkg/chat ./pkg/match
cd frontend; npm.cmd run build
cd ..
docker compose config
```

HTTP không mã hóa phù hợp cho lab/dev. Production nên bật TLS riêng vì Google OAuth, cookie và dữ liệu chat sẽ không an toàn khi truyền qua HTTP.
