# Docker Hub and Direct Helm Rollout to K3s

## Image tags

Workflow `.github/workflows/devsecops-platform.yml` builds these images:

```text
docker.io/tuananh165/nexuschat-api:<git-sha>
docker.io/tuananh165/nexuschat-web:<git-sha>
docker.io/tuananh165/nexuschat-ai-service:<git-sha>
docker.io/tuananh165/nexuschat-safety:<git-sha>
docker.io/tuananh165/nexuschat-discovery:<git-sha>
docker.io/tuananh165/nexuschat-workspace:<git-sha>
```

Lab proxy variants:

```text
docker.io/tuananh165/nexuschat-api:proxy-upload-<git-sha>
docker.io/tuananh165/nexuschat-web:proxy-upload-v2-<git-sha>
```

On pushes to `main`, the six primary images also receive the `latest` alias for Compose/lab convenience. Release deployments still use immutable Git SHA tags; do not use `latest` for production.

## Helm values

App chart:

```text
deployments/helm/nexuschat/
```

Values are merged in this order:

```text
values.yaml              # base/default
values-lab-k3s.yaml      # lab override
```

`values-lab-k3s.yaml` cannot run by itself; always pass it together with the chart and base values.

## Manual deployment

```bash
export TAG='<git-sha>'
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-k3s.yaml \
  --set-string imageDefaults.tag="$TAG" \
  --set-string services.web.image.fullname="docker.io/tuananh165/nexuschat-web:proxy-upload-v2-$TAG" \
  --set-string services.uploader.image.fullname="docker.io/tuananh165/nexuschat-api:proxy-upload-$TAG" \
  --wait --timeout 10m
```

## Platform before deploying the app

Stateful dependencies must exist first:

- Kafka Service `kafka.kafka.svc.cluster.local:9092`.
- Redis Cluster using the hostname in the lab values.
- Cassandra `cassandra.cassandra.svc.cluster.local:9042`.
- MinIO bucket `myfilebucket`.
- PostgreSQL database for the AI service.
- Traefik IngressClass `traefik`.

Observability/dashboard manifests:

```bash
kubectl apply -f deployments/platform/observability
kubectl apply -f deployments/platform/dashboards
```

Monitoring is installed separately. Create Grafana's required Secret first, then install kube-prometheus-stack with the lab values:

```bash
kubectl -n monitoring create secret generic grafana-admin \
  --from-literal=admin-user=admin \
  --from-literal=admin-password='<strong-random-password>' \
  --dry-run=client -o yaml | kubectl apply -f -
helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace \
  --values deployments/platform/monitoring/kube-prometheus-stack-values.yaml \
  --wait --timeout 10m
```

Prometheus/Grafana NodePorts are configured by:

```text
deployments/platform/monitoring/kube-prometheus-stack-values.yaml
```

## Dashboard NodePorts

```text
Kafka UI    : http://<NODE_IP>:30080
RedisInsight: http://<NODE_IP>:30540
Grafana     : http://<NODE_IP>:30300
Prometheus  : http://<NODE_IP>:30900
Jaeger      : http://<NODE_IP>:30686
```

NodePorts bypass Traefik. `nexuschat.click:<port>` works only if the domain resolves to the node IP and the firewall permits the port. It does not provide TLS automatically; production should use Traefik Ingress with subdomains and TLS.

## Cassandra schema during CD

The application Helm chart runs a pre-install/pre-upgrade Cassandra Job. It embeds `001_baseline.cql` followed by the versioned `002`–`010` migrations and records each successful file in `NexusChat.schema_migrations`. Migration `010_drop_messages_seen.cql` removes the obsolete per-message flag after old binaries are retired. A Git push must not run `cassandra/init.cql` manually on the server after the Helm Job is enabled. `init.cql` remains useful for standalone bootstrap and Compose; the migration files are required for upgrades and must be kept in source control.

## CI/CD

- Pull Request: tests, lint, builds, Helm rendering, and security scans.
- Push to `main`, `kafka`, or a `v*` tag: build, scan, generate an SBOM, and sign.
- Push to `main`: deploy Jaeger/OTel and dashboard manifests, then directly deploy the app to the lab with Helm.

The workflow triggers on `workflow_dispatch`, `pull_request` to any branch, pushes to `main` or `kafka`, and `v*` tags. The Kubernetes deploy job runs after the required image jobs succeed for either a push to `main` or a manual `workflow_dispatch`; it requires `build-images` and `build-proxy-variants` to pass, plus a self-hosted runner with labels `[self-hosted, linux, x64, k3s-lab]`.

CD runs on a self-hosted runner with these labels:

```text
[self-hosted, linux, x64, k3s-lab]
```

A `git pull` alone does not deploy; `git commit` followed by `git push` to `main` can trigger CD. The workflow does not automatically provision Kafka, Redis, Cassandra, MinIO, PostgreSQL, Traefik, kube-prometheus-stack, or Kyverno; prepare these dependencies separately before CD.

## Rollout/rollback

```bash
for deploy in web chat match user uploader forwarder ai-service safety discovery workspace notification; do
  kubectl -n nexuschat-lab rollout status deployment/$deploy --timeout=600s
done

helm history nexuschat -n nexuschat-lab
helm rollback nexuschat <REVISION> -n nexuschat-lab
```

