# NexusChat K8s Lab Dashboard Access

`<NODE_IP>` is the IP address of a Kubernetes node that accepts NodePort traffic. These dashboard ports are intended for a protected lab/private network.

## App access

| Service | URL |
| --- | --- |
| NexusChat via Traefik | `http://nexuschat.click` |
| NexusChat by node IP | `http://<NODE_IP>` |
| AI health (lab port-forward) | `http://127.0.0.1:18090/health` after `kubectl -n nexuschat-lab port-forward svc/ai-service 18090:8090` |

The `nexuschat.click` host is routed by Traefik to the app. The lab profile is HTTP-only; no TLS certificate is created.

## Direct NodePort URLs

| Dashboard | Direct URL | NodePort |
| --- | --- | ---: |
| Kafka UI | `http://<NODE_IP>:30080` | `30080` |
| RedisInsight | `http://<NODE_IP>:30540` | `30540` |
| Grafana | `http://<NODE_IP>:30300` | `30300` |
| Prometheus | `http://<NODE_IP>:30900` | `30900` |
| Jaeger | `http://<NODE_IP>:30686` | `30686` |
| MinIO API | `http://<NODE_IP>:30090` | `30090` |

If the DNS record for `nexuschat.click` resolves to the same node IP and the firewall permits the port, the equivalent form is `http://nexuschat.click:<PORT>`, for example `http://nexuschat.click:30300` for Grafana. This is still direct NodePort traffic: it bypasses Traefik and does not inherit TLS, authentication, or routing from the app host.

## Optional Traefik dashboard hosts

For host-based access through Traefik, first configure DNS records (or a private DNS wildcard) for:

```text
kafka.nexuschat.click
redis.nexuschat.click
minio.nexuschat.click
s3.nexuschat.click
grafana.nexuschat.click
prometheus.nexuschat.click
jaeger.nexuschat.click
```

Then apply the optional Ingress manifests:

```bash
kubectl apply -f deployments/platform/ingresses.yaml
kubectl apply -f deployments/platform/monitoring/standalone-monitoring.yaml
kubectl apply -f deployments/platform/monitoring/ingresses.yaml
```

The optional manifests use Traefik and the service names created by the current manifests. They do not create DNS or TLS. The monitoring Ingresses route to the lightweight standalone `grafana`, `prometheus`, and `jaeger-ui` services.

## Deployment sources

- Kafka UI and RedisInsight: `deployments/platform/dashboards/nodeport-dashboards.yaml`.
- Jaeger and OpenTelemetry Collector: `deployments/platform/observability/`.
- Grafana and Prometheus NodePorts: `deployments/platform/monitoring/standalone-monitoring.yaml`.
- Cassandra has no web dashboard; use CQL or `kubectl port-forward`.

## Security

Do not expose dashboard NodePorts directly to the public Internet. Use firewall allowlists, VPN/private networking and dashboard authentication. For production, prefer Traefik subdomains with TLS and an authenticated access layer. The NodePort manifests do not configure authentication for Kafka UI, RedisInsight, Prometheus or Jaeger.
