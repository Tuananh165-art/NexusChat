# DevSecOps platform plan hiện hành

## Image matrix

Workflow build/push/scan/SBOM/sign:

- `nexuschat-api`
- `nexuschat-web`
- `nexuschat-ai-service`
- `nexuschat-safety`
- `nexuschat-discovery`
- `nexuschat-workspace`

Mỗi image được tag bằng Git SHA hoặc release tag `v*`; không dùng mutable `latest`.

## Quality gates

1. Go unit và race test cho chat, match, realtime, Safety, Discovery, Workspace.
2. Frontend lint/build.
3. Docker build và Compose config.
4. Trivy vulnerability scan.
5. SPDX SBOM.
6. Cosign signature.
7. Helm upgrade `--atomic --wait`.
8. Smoke `/health` và `/ready`.

## Deployment matrix

Kubernetes chờ mười Deployment:

`web chat match user uploader forwarder ai-service safety discovery workspace`

K3s lab dùng ingress-nginx HTTP-only. `values-lab-k3s.yaml` đặt `tlsSecretName: ""`, tắt SSL redirect và không cài cert-manager/Coturn.

## Service contracts

- Chat gọi Safety gRPC trước khi broadcast.
- Match gọi Safety lọc block/risk và Discovery xếp hạng.
- Workspace kiểm tra channel membership qua chat authorization.
- Kafka topics dùng `nexuschat.*.events.v1`, outbox và `processed_events`.
- Redis giữ bounded waitlist, lock, cache, rate limit, reminder lease và fast dedup.
