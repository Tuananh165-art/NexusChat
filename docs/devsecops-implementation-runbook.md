# NexusChat DevSecOps Implementation Runbook

This runbook reflects the repository's current flow. Lab deployment uses a GitHub Actions self-hosted runner, Docker Hub, and direct Helm deployment into K3s. ArgoCD/GitOps is optional/reference material, not the default lab deployment path.

## 1. Repository preparation

1. Enable branch protection for `main`.
2. Require the `DevSecOps Platform Pipeline` workflow before merge.
3. Enable GitHub secret scanning and dependency alerts if the repository plan supports them.
4. Create a Docker Hub access token and store it as a GitHub Actions secret.
5. Do not commit `.env`, kubeconfig, tokens, OAuth secrets, JWT secrets, S3 secrets, database passwords, or AI provider keys.

Current required workflow permissions:

```yaml
contents: read
security-events: write
id-token: write
actions: read
```

## 2. Lab cluster bootstrap: K3s 4GB path

The K3s lab should use ingress-nginx, direct Helm deployment, and minimal dependencies. Do not install full ELK, Consul, kube-prometheus-stack, or ArgoCD on a 4GB machine unless there is a clear reason.

```bash
curl -sfL https://get.k3s.io | sudo INSTALL_K3S_EXEC="--disable traefik --write-kubeconfig-mode 644" sh -
mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown "$USER:$USER" ~/.kube/config
export KUBECONFIG=~/.kube/config
kubectl get nodes -o wide

curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

for ns in ingress-nginx nexuschat-lab redis kafka cassandra minio postgres monitoring argocd redis-ui; do
  kubectl create namespace "$ns" --dry-run=client -o yaml | kubectl apply -f -
done
```

Install lightweight ingress-nginx:

```bash
helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx \
  --set controller.replicaCount=1 \
  --set controller.metrics.enabled=false \
  --set controller.resources.requests.cpu=50m \
  --set controller.resources.requests.memory=96Mi \
  --set controller.resources.limits.cpu=300m \
  --set controller.resources.limits.memory=256Mi
kubectl -n ingress-nginx rollout status deployment/ingress-nginx-controller --timeout=180s
```

## 3. Stateful dependencies

The application Helm chart does not install Kafka, Redis, Cassandra, MinIO, or PostgreSQL. Install them separately and ensure endpoints match `values-lab-k3s.yaml`:

| Dependency | Expected endpoint in lab values |
| --- | --- |
| Kafka | `kafka.kafka.svc.cluster.local:9092` |
| Redis Cluster | `redis-redis-cluster-0/1/2.redis-redis-cluster-headless.redis.svc.cluster.local:6379` |
| Cassandra | `cassandra.cassandra.svc.cluster.local:9042` |
| MinIO | `http://minio.minio.svc.cluster.local:9000` |
| AI Postgres | `ai-postgres-postgresql.postgres.svc.cluster.local:5432` when using Bitnami chart |

Important: Go backend uses Redis Cluster client. Standalone Redis commonly fails with `ERR This instance has cluster support disabled`.

After Cassandra is ready, apply `cassandra/init.cql`. After MinIO is ready, create bucket `myfilebucket`.

See `docs/deploy-k8s-guide.md` for copy-paste dependency install commands.

## 4. Runtime secret `nexuschat-runtime`

Create in `nexuschat-lab` before app deploy:

```bash
kubectl create secret generic nexuschat-runtime \
  --namespace nexuschat-lab \
  --from-literal=CHAT_JWT_SECRET='change-me' \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --from-literal=CASSANDRA_USER='admin' \
  --from-literal=CASSANDRA_PASSWORD="$CASSANDRA_PASSWORD" \
  --from-literal=UPLOADER_S3_ACCESSKEY="$MINIO_ACCESS_KEY" \
  --from-literal=UPLOADER_S3_SECRETKEY="$MINIO_SECRET_KEY" \
  --from-literal=USER_OAUTH_GOOGLE_CLIENTID="$GOOGLE_CLIENT_ID" \
  --from-literal=USER_OAUTH_GOOGLE_CLIENTSECRET="$GOOGLE_CLIENT_SECRET" \
  --from-literal=NOTIFICATION_VAPID_PUBLICKEY="$NOTIFICATION_VAPID_PUBLICKEY" \
  --from-literal=NOTIFICATION_VAPID_PRIVATEKEY="$NOTIFICATION_VAPID_PRIVATEKEY" \
  --from-literal=NOTIFICATION_VAPID_SUBJECT='mailto:admin@nexuschat.click' \
  --from-literal=CALL_TURN_SHAREDSECRET="$CALL_TURN_SHAREDSECRET" \
  --from-literal=DATABASE_URL="postgresql+asyncpg://nexuschat_ai:$AI_POSTGRES_PASSWORD@ai-postgres-postgresql.postgres.svc.cluster.local:5432/nexuschat_ai" \
  --from-literal=AI_ENDPOINT="$AI_ENDPOINT" \
  --from-literal=AI_API_KEY="$AI_API_KEY" \
  --from-literal=AI_MODEL="$AI_MODEL" \
  --from-literal=AI_POSTGRES_PASSWORD="$AI_POSTGRES_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Use placeholders only for pods to start in a private lab; AI/OAuth features will not work until real values are set.

## 5. GitHub Actions setup

Repository secrets:

- `DOCKER_USERNAME`
- `DOCKER_PASSWORD`
- `KUBE_CONFIG_B64` unless the self-hosted runner already has a working kubeconfig.

Create kubeconfig secret:

```bash
base64 -w0 ~/.kube/config
```

Current deploy job requires a self-hosted runner with labels:

```text
self-hosted, linux, x64, k3s-lab
```

This is correct for a private LAN K3s cluster. If you switch to GitHub-hosted runners, the Kubernetes API server must be reachable from GitHub.

## 6. CI/CD execution path

On PR:

1. Go `make test`.
2. Frontend `npm ci`, `npm run lint`, `npm run build`.
3. AI `python -m pip install -e ".[dev]"`, `ruff`, `pytest`.
4. Helm lint/render default + lab profile.
5. Gitleaks, dependency review, CodeQL, Trivy FS.

On push to `main`:

1. Same validation gates.
2. Build/push/sign/scan/SBOM primary Docker Hub images.
3. Build/push proxy/lab variant images.
4. Deploy lab with Helm directly.
5. Wait rollout for deployment names: `web chat match user uploader forwarder ai-service presence notification call`; verify Coturn when enabled.

## 7. Manual lab deploy equivalent

```bash
export TAG='<git-sha-or-existing-image-tag>'
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

If you deploy a non-proxy tag manually, remove the two `services.*.image.fullname` overrides or set them to existing image tags.

## 8. Post-deploy verification

```bash
kubectl -n nexuschat-lab get pods -o wide
kubectl -n nexuschat-lab get svc
kubectl -n nexuschat-lab get ingress
kubectl -n nexuschat-lab get deploy -o jsonpath='{range .items[*]}{.metadata.name}{" => "}{range .spec.template.spec.containers[*]}{.image}{" "}{end}{"\n"}{end}'

for deploy in web chat match user uploader forwarder ai-service presence notification call; do
  kubectl -n nexuschat-lab rollout status deployment/$deploy --timeout=600s
done

curl -I https://nexuschat.click
curl -i http://IP/api/ai/health
```

If using `nip.io` lab hostnames from `deployments/platform/UI-ACCESS.md`, also check dashboard ingresses.

## 9. Optional platform components

Apply only when the cluster has enough resources:

| Component | Path |
| --- | --- |
| ArgoCD values/apps | `deployments/platform/argocd`, `deployments/gitops/applications` |
| ingress-nginx values | `deployments/platform/ingress-nginx/values.yaml` |
| Monitoring | `deployments/platform/monitoring/*` |
| Logging/ELK | `deployments/platform/logging/*` |
| Kyverno policies | `deployments/platform/security/kyverno-policies.yaml` |
| Consul | `deployments/platform/consul/values.yaml` |

ArgoCD applications in repo are optional examples. Current lab CD does not require ArgoCD sync or GitOps image-tag commits.

## 10. Rollback

```bash
helm history nexuschat -n nexuschat-lab
helm rollback nexuschat <REVISION> -n nexuschat-lab

for deploy in web chat match user uploader forwarder ai-service; do
  kubectl -n nexuschat-lab rollout status deployment/$deploy --timeout=600s
done
```

Or redeploy old immutable image tags:

```bash
export OLD_TAG='<previous-sha>'
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-k3s.yaml \
  --set-string imageDefaults.tag="$OLD_TAG" \
  --set-string services.web.image.fullname="docker.io/tuananh165/nexuschat-web:proxy-upload-v2-$OLD_TAG" \
  --set-string services.uploader.image.fullname="docker.io/tuananh165/nexuschat-api:proxy-upload-$OLD_TAG" \
  --wait --timeout 10m
```

## 11. Production promotion

Do not rely on mutable `latest`. Promote an approved SHA or `v*` tag:

```bash
export IMAGE_TAG='<approved-sha-or-v-tag>'
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-prod \
  --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --set-string global.environment=production \
  --set-string global.domain=nexuschat.click \
  --set-string services.user.env.USER_AUTH_COOKIE_DOMAIN=nexuschat.click \
  --set-string services.user.env.USER_OAUTH_COOKIE_DOMAIN=nexuschat.click \
  --set-string services.uploader.env.UPLOADER_S3_PUBLICENDPOINT=https://assets.nexuschat.click \
  --set-string imageDefaults.tag="$IMAGE_TAG" \
  --wait --timeout 10m
```

Before production cutover: TLS secret, DNS, OAuth redirect URIs, CORS/cookie domains, runtime secrets, storage bucket, observability, rollback tag, and smoke tests must be confirmed.
