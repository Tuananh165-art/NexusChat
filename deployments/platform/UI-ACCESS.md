# NexusChat K8s Lab Dashboard URLs

Lab base IP: `192.168.109.131`.

Các URL dùng `nip.io`, không cần sửa `/etc/hosts`: mọi hostname dạng `*.192.168.109.131.nip.io` resolve về `192.168.109.131`.

## NexusChat app

| Service | URL |
| --- | --- |
| Web app by IP | `http://192.168.109.131` |
| AI health by IP | `http://192.168.109.131/api/ai/health` |

Nếu deploy với domain `nexuschat.click`, app ingress host là domain đó thay vì hostless/IP.

## Platform/dashboard URLs

| Service | URL | Notes |
| --- | --- | --- |
| Kafka UI | `http://kafka.192.168.109.131.nip.io` | Requires a `kafka-ui` service in namespace `kafka` |
| RedisInsight | `http://redis.192.168.109.131.nip.io` | Requires `redisinsight` service in namespace `redis-ui` |
| MinIO Console | `http://minio.192.168.109.131.nip.io` | Credentials depend on your lab install, commonly `labminio / lab-minio-secret` |
| MinIO/S3 API | `http://s3.nexuschat.click` | Current ingress manifest uses this host for MinIO API |
| Prometheus | `http://prometheus.192.168.109.131.nip.io` | Requires monitoring manifests/services |
| Grafana | `http://grafana.192.168.109.131.nip.io` | Credentials depend on installed values, do not commit real password |
| Jaeger | `http://jaeger.192.168.109.131.nip.io` | Requires Jaeger service in namespace `monitoring` |
| ArgoCD | `http://argocd.192.168.109.131.nip.io` | Optional; not required for current lab CD path |
| Cassandra | no web UI | Use port-forward/CQL: `kubectl -n cassandra port-forward svc/cassandra 9042:9042` |

## Config files

- `deployments/platform/ingresses.yaml`: Kafka UI, RedisInsight, MinIO API/console, Jaeger, ArgoCD ingress references.
- `deployments/platform/monitoring/ingresses.yaml`: Grafana, Prometheus, Jaeger ingress references.
- `deployments/platform/monitoring/standalone-monitoring.yaml`: standalone Prometheus/Grafana references.

Apply only the manifests for services that actually exist in the cluster. Ingress objects pointing to missing Services will be created but will return 503.
