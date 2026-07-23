# NexusChat K8s Lab Dashboard URLs

Lab base IP: `IP`.

These URLs use `nip.io`; no `/etc/hosts` changes are required. Any hostname matching `*.IP.nip.io` resolves to `IP`.

## NexusChat app

| Service | URL |
| --- | --- |
| Web app by IP | `http://IP` |
| AI health by IP | `http://IP/api/ai/health` |

If deploying with the `nexuschat.click` domain, the app ingress host is that domain instead of a hostless/IP ingress.

## Platform/dashboard URLs

| Service | URL | Notes |
| --- | --- | --- |
| Kafka UI | `http://kafka.IP.nip.io` | Requires a `kafka-ui` service in namespace `kafka` |
| RedisInsight | `http://redis.IP.nip.io` | Requires `redisinsight` service in namespace `redis-ui` |
| MinIO Console | `http://minio.IP.nip.io` | Credentials depend on your lab install, commonly `labminio / lab-minio-secret` |
| MinIO/S3 API | `http://s3.nexuschat.click` | Current ingress manifest uses this host for MinIO API |
| Prometheus | `http://prometheus.IP.nip.io` | Requires monitoring manifests/services |
| Grafana | `http://grafana.IP.nip.io` | Credentials depend on installed values, do not commit real password |
| Jaeger | `http://jaeger.IP.nip.io` | Requires Jaeger service in namespace `monitoring` |
| ArgoCD | `http://argocd.IP.nip.io` | Optional; not required for current lab CD path |
| Cassandra | no web UI | Use port-forward/CQL: `kubectl -n cassandra port-forward svc/cassandra 9042:9042` |

## Config files

- `deployments/platform/ingresses.yaml`: Kafka UI, RedisInsight, MinIO API/console, Jaeger, ArgoCD ingress references.
- `deployments/platform/monitoring/ingresses.yaml`: Grafana, Prometheus, Jaeger ingress references.
- `deployments/platform/monitoring/standalone-monitoring.yaml`: standalone Prometheus/Grafana references.

Apply only the manifests for services that actually exist in the cluster. Ingress objects pointing to missing Services will be created but will return 503.
