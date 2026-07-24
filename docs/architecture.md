# NexusChat Architecture

NexusChat hiện là hệ thống chat realtime additive gồm các Go service hiện hữu và ba service nghiệp vụ mới. Runtime lab chạy HTTP/`ws://` theo yêu cầu, không có TLS, Web Push hoặc WebRTC.

## Service boundaries

| Service | State | Internal contract |
|---|---|---|
| chat | Cassandra messages/channels, Redis sessions | Kafka chat events, Safety gRPC |
| match | Redis waitlist | Chat gRPC, Safety gRPC, Discovery gRPC |
| safety | Cassandra decisions/reports/rules, Redis risk/rate-limit/block cache | Kafka safety events |
| discovery | Cassandra profile/match/feedback, Redis score/cache | Kafka discovery events |
| workspace | Cassandra item/collection, Redis board/reminder/lock | Kafka workspace events, Chat authorization |
| user | Redis sessions, Cassandra/user state | user gRPC |
| forwarder | Redis routing | Kafka fanout |
| ai-service | PostgreSQL/Alembic | HTTP optional semantic enrichment |

## Safety flow

`chat.BroadcastTextMessage` gọi Safety gRPC trước khi lưu/broadcast. Rule engine tính risk score:

- 0–39: allow
- 40–69: warn/che nội dung
- 70–100: block

Flood, repeat spam, scam pattern, toxic pattern và block list dùng Redis. Quyết định cảnh báo/chặn được lưu Cassandra và publish `nexuschat.safety.events.v1`. Khi Safety timeout, chat giữ degraded mode bằng local hard-rule và vẫn phát event để xử lý lại.

## Discovery flow

`match` lấy candidate từ Redis, lọc block/risk rồi gọi Discovery gRPC. Điểm xếp hạng gồm interest overlap, language, conversation goal, reputation và thời gian chờ. Pair lock dùng Redis; nếu Discovery lỗi, FIFO matching cũ được dùng làm fallback.

## Workspace flow

Workspace xác thực channel membership qua token Cassandra. Task/note/bookmark được lưu hai projection Cassandra theo channel và item ID. Mọi thay đổi phát Kafka và gửi WebSocket đến client. Reminder dùng Redis sorted set + lease để nhiều replica không xử lý trùng; dữ liệu quá hạn vẫn được đọc từ Cassandra.

## Kafka reliability

Envelope gồm `event_id`, `event_type`, `schema_version`, `occurred_at`, `producer`, `aggregate_id`, correlation/causation ID và payload. Consumer dùng `processed_events`, bounded worker pool, retry backoff, outbox relay và DLQ. Offset chỉ commit sau khi handler nghiệp vụ thành công.

## Data migration

`cassandra/migrations/002_replace_realtime_services.cql` drop:

- `notifications_by_user`
- `push_subscriptions_by_user`
- `notification_preferences`
- `calls_by_user`
- `calls_by_channel`

Giữ `processed_events` và `outbox_events` cho service mới. Chỉ chạy migration sau khi backup keyspace.

## Deployment

Helm chart dùng ingress-nginx HTTP-only:

- `global.tlsSecretName: ""`
- `nginx.ingress.kubernetes.io/ssl-redirect: "false"`
- `nexuschat.click`
- `/api/safety`
- `/api/discovery`
- `/api/workspace`

Ba image runtime mới:

- `docker.io/tuananh165/nexuschat-safety:<sha>`
- `docker.io/tuananh165/nexuschat-discovery:<sha>`
- `docker.io/tuananh165/nexuschat-workspace:<sha>`

## Security note

HTTP-only không phù hợp production: token, cookie và nội dung chat có thể bị nghe lén; Google OAuth callback và các browser secure-context API cũng có thể bị giới hạn. Cấu hình hiện tại chỉ đáp ứng lab/dev không TLS.
