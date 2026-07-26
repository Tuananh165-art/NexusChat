# NexusChat Kubernetes/K3s Lab Deployment Guide

This document describes the current NexusChat Kubernetes lab deployment on K3s with approximately 4GB RAM/50GB disk. The current model uses Traefik, direct Helm deployment, Prometheus/Grafana, Jaeger/OpenTelemetry Collector, and Kyverno. ArgoCD, Consul, ECK/ELK, and ingress-nginx are not used.

## 1. Distinguish the Helm files

`deployments/helm/nexuschat/values-lab-k3s.yaml` **is not an independent deployment file**. It is the lab override file. Helm must use the chart, the default values, and the lab values:

```text
deployments/helm/nexuschat/Chart.yaml
  ├── values.yaml                 # base/staging-like configuration
  ├── values-lab-k3s.yaml         # override lab
  └── templates/*.yaml             # Deployment, Service, Ingress, policy...
```

Standard deployment command:

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-k3s.yaml \
  --set-string imageDefaults.tag="$GIT_SHA" \
  --wait --timeout 10m
```

Helm merges values in order; the lab file is applied afterward and overrides the base file. The app chart creates only NexusChat workloads; it does not install Kafka, Redis, Cassandra, MinIO, PostgreSQL, Prometheus, Grafana, Jaeger, Traefik, or Kyverno.

## 2. Current lab profile

- App namespace: `nexuschat-lab`.
- Monitoring namespace: `monitoring`.
- Ingress class: `traefik`.
- App host: `nexuschat.click`.
- TLS: disabled in the lab; `global.tlsSecretName` is empty and cert-manager is not used.
- Deployment strategy: `Recreate` with one replica for the lab profile.
- App tracing: disabled in `values-lab-k3s.yaml`; the Collector can still run for workloads configured for OTLP.
- App chart ServiceMonitor and NetworkPolicy: disabled in the lab profile.

DNS `nexuschat.click` must point to the node/Traefik IP. Helm does not create DNS records, firewall rules, or TLS certificates.

## 3. Install K3s and Helm

If installing a new K3s cluster, keep the K3s default Traefik:

```bash
curl -sfL https://get.k3s.io | sudo sh -
mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown "$USER:$USER" ~/.kube/config
export KUBECONFIG=~/.kube/config
kubectl get nodes -o wide
kubectl get pods -n kube-system
kubectl get ingressclass
```

Install Helm and repositories:

```bash
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add kyverno https://kyverno.github.io/kyverno/
helm repo update
helm version
```

Do not install `ingress-nginx`; Traefik is the only Ingress controller.

## 4. Create namespaces

```bash
for ns in nexuschat-lab monitoring kafka redis redis-ui cassandra minio postgres; do
  kubectl create namespace "$ns" --dry-run=client -o yaml | kubectl apply -f -
done
```

## 5. Install runtime dependencies

The app requires Kafka, Redis Cluster, Cassandra, MinIO, and PostgreSQL for the AI service. Helm or external services can be used. Before deploying the app, verify the DNS names used by the values:

| Dependency | Host used by the lab profile |
|---|---|
| Kafka | `kafka.kafka.svc.cluster.local:9092` |
| Redis Cluster | `redis-redis-cluster-0/1/2.redis-redis-cluster-headless.redis.svc.cluster.local:6379` |
| Cassandra | `cassandra.cassandra.svc.cluster.local:9042` |
| MinIO | `minio.minio.svc.cluster.local:9000` |
| AI service Redis (optional integration) | `redis-redis-cluster-0.redis-redis-cluster-headless.redis.svc.cluster.local:6379` |
### Redis Cluster

Go services use a Redis Cluster client, not standalone Redis. The AI service currently accepts only `REDIS_URL` configuration and does not require Redis to start; if Redis integration is enabled, the lab values point to the first Redis Cluster member. If the AI deployment uses a library that requires standalone Redis, provision an additional standalone Redis and override `services.ai-service.env.REDIS_URL`.

```bash
export REDIS_PASSWORD='change-this-lab-password'
helm upgrade --install redis bitnami/redis-cluster \
  --namespace redis \
  --set image.repository=bitnamilegacy/redis-cluster \
  --set image.tag=8.2.1 \
  --set usePassword=true \
  --set password="$REDIS_PASSWORD" \
  --set auth.password="$REDIS_PASSWORD" \
  --set cluster.nodes=3 \
  --set cluster.replicas=0 \
  --set persistence.enabled=true \
  --set persistence.size=1Gi
kubectl -n redis get pods,svc -o wide
```

### Kafka

Kafka must provide a Service named `kafka` in the `kafka` namespace and listen on `9092`. A single-node KRaft StatefulSet can be used for the lab, or a controlled Kafka operator. Check it with:

```bash
kubectl -n kafka get pods,svc
kubectl -n kafka get svc kafka
```

### Cassandra

The lab manifest can be applied:

```bash
kubectl apply -f deployments/platform/cassandra/cassandra.yaml
kubectl -n cassandra get pods,svc,pvc
```

After Cassandra is Ready, apply `cassandra/init.cql` and the migration `cassandra/migrations/002_replace_realtime_services.cql` according to the project backup/schema procedure. The migration deletes old tables; do not run it before backing up the keyspace.

### MinIO and PostgreSQL

MinIO requires the `myfilebucket` bucket. PostgreSQL requires the `nexuschat_ai` database and a user/password matching `DATABASE_URL`. Bitnami Helm or a managed service can be used, but update `nexuschat-runtime` and the hostnames in the values if the Service name is different.

## 6. Install monitoring and tracing

### Prometheus and Grafana

Create the Grafana Secret before installing the chart. The values only reference the Secret and do not contain the password:

```bash
kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -
kubectl -n monitoring create secret generic grafana-admin \
  --from-literal=admin-user=admin \
  --from-literal=admin-password='<strong-random-password>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

`kube-prometheus-stack-values.yaml` contains values for the Kubernetes monitoring chart. The current profile targets a lab of approximately 4GB/50GB:

- Grafana NodePort `30300`.
- Prometheus NodePort `30900`.
- One replica for Prometheus and Alertmanager.
- Small retention and PVC settings to reduce resource usage.
- Grafana persistence.

Production requires separate values with appropriate HA, storage, and retention.

Install the stack:

```bash
helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --values deployments/platform/monitoring/kube-prometheus-stack-values.yaml \
  --wait --timeout 10m
```

Check Service names:

```bash
kubectl -n monitoring get pods,svc
```

For the release name `kube-prometheus-stack`, the monitoring Ingress points to:

```text
kube-prometheus-stack-grafana
kube-prometheus-stack-prometheus
```

### Jaeger and OpenTelemetry Collector

Apply the manifest:

```bash
kubectl apply -f deployments/platform/observability
kubectl -n monitoring get pods,svc
```

Main Services:

- `jaeger.monitoring.svc.cluster.local:16686`: internal Jaeger UI.
- `jaeger.monitoring.svc.cluster.local:4317`: Jaeger OTLP gRPC.
- `otel-collector.monitoring.svc.cluster.local:4317`: Collector OTLP gRPC.
- `otel-collector.monitoring.svc.cluster.local:4318`: Collector OTLP HTTP.

The Jaeger UI is exposed through the `jaeger-ui` Service at NodePort `30686`. The OpenTelemetry Collector does not need a NodePort; applications in the cluster send traces to the internal Service.

### Kyverno

Kyverno is a Kubernetes admission/policy controller and does not run in Docker Compose:

```bash
helm upgrade --install kyverno kyverno/kyverno \
  --namespace security \
  --create-namespace \
  --wait --timeout 10m
kubectl apply -f deployments/platform/security/kyverno-policies.yaml
kubectl get clusterpolicy
```

Check `validationFailureAction` before changing policies from Audit to Enforce. The current policies mainly apply to staging/prod namespaces; expand the namespace match if enforcement is required for `nexuschat-lab`.

## 7. Kafka UI and RedisInsight

Kafka and Redis do not have integrated web dashboards. The following manifest deploys both UIs:

```bash
kubectl apply -f deployments/platform/dashboards/nodeport-dashboards.yaml
kubectl -n kafka get deployment,svc kafka-ui
kubectl -n redis-ui get deployment,svc redisinsight
```

NodePorts:

| UI | Direct URL |
|---|---|
| Kafka UI | `http://<NODE_IP>:30080` |
| RedisInsight | `http://<NODE_IP>:30540` |

Kafka UI connects to `kafka.kafka.svc.cluster.local:9092` by default. RedisInsight requires a Redis connection to be entered in the interface if no connection profile is present.

Dashboard NodePorts provide direct access to the node and do not pass through Traefik. Do not expose these ports to the Internet without firewall/VPN/authentication controls.

## 8. Dashboard domains and Traefik

Current app domain:

```text
http://nexuschat.click
```

This is Traefik Host routing for the app, not a NodePort. With DNS `nexuschat.click` pointing to the node IP, the following URLs can access NodePorts:

```text
http://nexuschat.click:30300   # Grafana
http://nexuschat.click:30900   # Prometheus
http://nexuschat.click:30686   # Jaeger
http://nexuschat.click:30080   # Kafka UI
http://nexuschat.click:30540   # RedisInsight
```

However, NodePorts bypass Traefik and do not automatically inherit the domain's TLS. The standard production approach is to create separate DNS records and Traefik Ingresses:

```text
grafana.nexuschat.click
prometheus.nexuschat.click
jaeger.nexuschat.click
kafka.nexuschat.click
redis.nexuschat.click
```

The existing dashboard manifests use `*.nexuschat.click` production/lab hosts; replace the DNS with real or private DNS records before applying them. NodePorts can be used directly without these Ingresses. To use host routing through Traefik:

```bash
kubectl apply -f deployments/platform/ingresses.yaml
kubectl apply -f deployments/platform/monitoring/ingresses.yaml
```

The dashboard hosts are `grafana.nexuschat.click`, `prometheus.nexuschat.click`, `jaeger.nexuschat.click`, `kafka.nexuschat.click`, and `redis.nexuschat.click`. The manifests do not create DNS or TLS; the lab should continue using NodePort URLs if DNS/subdomains are not configured.

## 9. Runtime secret

Create the Secret before installing Helm. Do not commit a real password to the values or repository:

```bash
kubectl create secret generic nexuschat-runtime \
  --namespace nexuschat-lab \
  --from-literal=CHAT_JWT_SECRET='change-me' \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --from-literal=CASSANDRA_USER='admin' \
  --from-literal=CASSANDRA_PASSWORD='change-me' \
  --from-literal=UPLOADER_S3_ACCESSKEY='change-me' \
  --from-literal=UPLOADER_S3_SECRETKEY='change-me' \
  --from-literal=USER_OAUTH_GOOGLE_CLIENTID='change-me' \
  --from-literal=USER_OAUTH_GOOGLE_CLIENTSECRET='change-me' \
  --from-literal=DATABASE_URL='postgresql+asyncpg://user:password@postgres.postgres.svc.cluster.local:5432/nexuschat_ai' \
  --dry-run=client -o yaml | kubectl apply -f -
```

Add AI/provider keys from `ai-service/README.md` if using AI. Rotate every credential that has ever been used in a local/dev file.

## 10. Deploy the application

Deploy manually with an immutable tag:

```bash
export TAG='<git-sha>'
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-k3s.yaml \
  --set-string imageDefaults.tag="$TAG" \
  --set-string services.web.image.fullname="docker.io/tuananh165/nexuschat-web:proxy-upload-v2-$TAG" \
  --set-string services.uploader.image.fullname="docker.io/tuananh165/nexuschat-api:proxy-upload-$TAG" \
  --wait --timeout 10m
```

Do not use `latest` for a release deployment. Proxy images must exist with the exact tag before overriding `web`/`uploader`.

## 11. Verify

```bash
kubectl -n nexuschat-lab get pods,svc,ingress
kubectl -n monitoring get pods,svc
kubectl -n kafka get pods,svc
kubectl -n redis-ui get pods,svc
kubectl get ingressclass

for deploy in web chat match user uploader forwarder ai-service safety discovery workspace; do
  kubectl -n nexuschat-lab rollout status deployment/$deploy --timeout=600s
done

curl -i http://nexuschat.click
curl -i http://nexuschat.click/api/ai/health
```

Check NodePorts:

```bash
curl -I http://<NODE_IP>:30300
curl -I http://<NODE_IP>:30900
curl -I http://<NODE_IP>:30686
curl -I http://<NODE_IP>:30080
curl -I http://<NODE_IP>:30540
```

Check traces/metrics:

```bash
kubectl -n monitoring logs deploy/otel-collector --tail=100
kubectl -n monitoring logs deploy/jaeger --tail=100
kubectl -n monitoring get svc jaeger jaeger-ui otel-collector
```

## 12. GitHub Actions CD

Workflow `.github/workflows/devsecops-platform.yml` is the direct CD source of truth:

- Pull Request: tests, lint, builds, Helm rendering, and security scans.
- Push to `main`, `kafka`, or a `v*` tag: build images, blocking Trivy scans, SBOM generation, and Cosign signing.
- Push to `main`: apply Jaeger/OTel and Kafka UI/RedisInsight manifests, then deploy the app with Helm into `nexuschat-lab`.
- ArgoCD is not used.

The workflow triggers on `workflow_dispatch`, `pull_request` to any branch, pushes to `main` or `kafka`, and `v*` tags. The Kubernetes deploy job runs only after a successful push to `main`, requires `build-images` and `build-proxy-variants` to pass, and requires a self-hosted runner with labels `[self-hosted, linux, x64, k3s-lab]`.

The deploy runner must have these labels:

```text
[self-hosted, linux, x64, k3s-lab]
```

Minimum secrets:

```text
DOCKER_USERNAME
DOCKER_PASSWORD
KUBE_CONFIG_B64
```

`KUBE_CONFIG_B64` must point to the correct cluster. Do not rely on an old self-hosted runner kubeconfig in production.

A `git pull` alone does not deploy; `git commit` followed by `git push` to `main` can trigger CD. The workflow does not automatically provision Kafka, Redis, Cassandra, MinIO, PostgreSQL, Traefik, kube-prometheus-stack, or Kyverno; those dependencies must be prepared separately and be Ready before CD.

## 13. Rollback and cleanup

```bash
helm history nexuschat -n nexuschat-lab
helm rollback nexuschat <REVISION> -n nexuschat-lab
helm uninstall nexuschat -n nexuschat-lab
```

Remove observability/dashboard resources separately if needed:

```bash
kubectl delete -f deployments/platform/dashboards/nodeport-dashboards.yaml
kubectl delete -f deployments/platform/observability
helm uninstall kube-prometheus-stack -n monitoring
```

Do not delete Cassandra, Redis, PostgreSQL, MinIO, or Kafka PVCs without confirming the backup.

## 14. Troubleshooting

### Ingress does not route

```bash
kubectl get ingressclass
kubectl -n nexuschat-lab describe ingress
kubectl -n kube-system get pods | grep -i traefik
```

Ensure `ingressClassName: traefik`, DNS points to the node IP, and the Service has endpoints.

### NodePort is unreachable

```bash
kubectl -n monitoring get svc
kubectl -n kafka get svc kafka-ui
kubectl -n redis-ui get svc redisinsight
sudo ufw status
```

Open ports `30080`, `30300`, `30540`, `30686`, and `30900` in the lab network as required; do not expose them publicly without controls.

### Redis connection error

Verify that Redis is actually running as a Cluster and that the hostname in `values-lab-k3s.yaml` matches the Service/headless Service:

```bash
kubectl -n redis get pods,svc
```

### Kafka UI cannot see the broker

Check the Service and advertised listener:

```bash
kubectl -n kafka get svc kafka
kubectl -n kafka logs statefulset/kafka --tail=100
```

`KAFKA_ADVERTISED_LISTENERS` must advertise a hostname that Kafka UI and applications in the cluster can resolve.
