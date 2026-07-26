# NexusChat Architecture

NexusChat is a Go/Next.js real-time chat system with a FastAPI AI service. The Kubernetes lab runs HTTP/`ws://` through Traefik; production requires TLS, secret management, and stricter network policies.

## Runtime flow

```text
Browser
  │ HTTP + WebSocket
  ▼
Traefik Ingress
  ├── web
  ├── chat ── Safety gRPC
  ├── match ── Discovery/Safety gRPC
  ├── user
  ├── uploader ── MinIO + ForwardAuth
  └── ai-service ── PostgreSQL

Go services ── Kafka + Redis Cluster + Cassandra
Go/Traefik traces ── OpenTelemetry Collector ── Jaeger
Go/Python metrics ── Prometheus ── Grafana
```

## Service boundaries

| Service | State | Internal contract |
|---|---|---|
| chat | Cassandra messages/channels, Redis sessions | Kafka events, Safety gRPC |
| match | Redis waitlist | Chat, Safety, Discovery gRPC |
| safety | Cassandra decisions/reports/rules, Redis risk/rate-limit/block cache | Kafka safety events |
| discovery | Cassandra profile/match/feedback, Redis score/cache | Kafka discovery events |
| workspace | Cassandra items/collections, Redis board/reminder/lock | Kafka events, Chat authorization |
| user | Redis sessions and user state | User gRPC |
| forwarder | Redis routing | Kafka fanout |
| ai-service | PostgreSQL/Alembic | Optional HTTP semantic enrichment |

## Platform responsibilities

| Component | Local Compose | Kubernetes lab |
|---|---|---|
| Ingress | Traefik container `reverse-proxy` | Traefik IngressClass `traefik` |
| Metrics | Prometheus container | kube-prometheus-stack |
| Dashboards | Grafana container | kube-prometheus-stack Grafana NodePort |
| Traces | Jaeger + OTel Collector | Jaeger/OTel manifests in `deployments/platform/observability` |
| Kafka dashboard | Not available by default | Kafka UI NodePort `30080` |
| Redis dashboard | Not available by default | RedisInsight NodePort `30540` |
| Policy | Not applied | Kyverno |

## Data flow

Chat calls Safety before saving or broadcasting. Match filters blocks/risk and then calls Discovery to rank candidates. Workspace verifies channel membership through Chat. Kafka uses versioned envelopes, outbox, `processed_events`, retries, and a DLQ; Redis provides caching, locking, rate limiting, waitlists, and deduplication; Cassandra retains long-term business data.

## Helm deployment model

`deployments/helm/nexuschat/values.yaml` is the base values file. `values-lab-k3s.yaml` is the lab override and must be passed after the base values file:

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  -n nexuschat-lab --create-namespace \
  -f deployments/helm/nexuschat/values.yaml \
  -f deployments/helm/nexuschat/values-lab-k3s.yaml
```

The Helm chart deploys only application workloads. Kafka, Redis, Cassandra, MinIO, PostgreSQL, kube-prometheus-stack, and Kyverno are platform dependencies installed separately.

## Dashboard URL model

NodePorts provide direct access through the node IP:

```text
Kafka UI     : <NODE_IP>:30080
RedisInsight : <NODE_IP>:30540
Grafana      : <NODE_IP>:30300
Prometheus   : <NODE_IP>:30900
Jaeger       : <NODE_IP>:30686
```

`nexuschat.click:<port>` works only when the domain DNS points to the node IP and the firewall allows the port. NodePorts bypass Traefik and do not provide TLS automatically. The standard production approach is to use subdomains with Traefik Ingress and TLS.

## Security note

HTTP-only and public NodePorts are not suitable for production: tokens, cookies, chat content, and dashboards could be accessed without authorization. Production requires TLS, dashboard authentication, VPN/allowlisting, Kyverno Enforce, image signature verification, and a secret manager.
