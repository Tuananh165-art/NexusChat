# NexusChat K8s Lab Dashboard Access

`<NODE_IP>` is the IP address of a Kubernetes node that accepts NodePort traffic. These dashboard ports are intended for a protected lab/private network.

## App access

| Service | URL |
| --- | --- |
| NexusChat via Traefik | `http://nexuschat.click` |
| NexusChat by node IP | `http://<NODE_IP>` |
| AI health through app ingress | `http://nexuschat.click/api/ai/health` |

The `nexuschat.click` host is routed by Traefik to the app. The lab profile is HTTP-only; no TLS certificate is created.

## Direct NodePort URLs

| Dashboard | Direct URL | NodePort |
| --- | --- | ---: |
| Kafka UI | `http://<NODE_IP>:30080` | `30080` |
| RedisInsight | `http://<NODE_IP>:30540` | `30540` |
| Grafana | `http://<NODE_IP>:30300` | `30300` |
| Prometheus | `http://<NODE_IP>:30900` | `30900` |
| Jaeger | `http://<NODE_IP>:30686` | `30686` |

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
kubectl apply -f deployments/platform/monitoring/ingresses.yaml
```

The optional manifests use Traefik and the service names created by the current manifests/Helm release. They do not create DNS or TLS. The monitoring Ingresses require a kube-prometheus-stack release named `kube-prometheus-stack`.

## Deployment sources

- Kafka UI and RedisInsight: `deployments/platform/dashboards/nodeport-dashboards.yaml`.
- Jaeger and OpenTelemetry Collector: `deployments/platform/observability/`.
- Grafana and Prometheus NodePorts: `deployments/platform/monitoring/kube-prometheus-stack-values.yaml`.
- Cassandra has no web dashboard; use CQL or `kubectl port-forward`.

## Security

Do not expose dashboard NodePorts directly to the public Internet. Use firewall allowlists, VPN/private networking and dashboard authentication. For production, prefer Traefik subdomains with TLS and an authenticated access layer. The NodePort manifests do not configure authentication for Kafka UI, RedisInsight, Prometheus or Jaeger.
