# NexusChat K3s Lab Deployment Guide (4GB RAM / 50GB disk)

This runbook deploys the lab environment represented by the current source. Production can use the same Helm chart, but it needs stronger resource sizing, secrets, TLS, DNS, storage, and observability.

Current target lab:

- Server/K3s ingress IP: `192.168.109.131`.
- Namespace app: `nexuschat-lab`.
- Ingress controller: ingress-nginx.
- App Helm release: `nexuschat`.
- App chart: `deployments/helm/nexuschat`.
- Lab values: `deployments/helm/nexuschat/values-lab-k3s.yaml`.
- Registry: Docker Hub `docker.io/tuananh165`.
- Current CI/CD: GitHub Actions self-hosted runner + direct Helm deploy, not ArgoCD.

## 0. What not to run on a 4GB lab by default

Do not install these on the small lab unless you have confirmed free RAM/disk:

- Full ELK/ECK/Elasticsearch/Kibana/Filebeat.
- Full kube-prometheus-stack.
- Consul service mesh.
- HA Kafka/Cassandra/Redis/Postgres/MinIO.
- ArgoCD as part of the critical deploy path.

The app chart also does not install stateful dependencies. Install dependencies separately, or use managed/external services and override Helm values.

## 1. Install K3s without bundled Traefik

```bash
curl -sfL https://get.k3s.io | sudo INSTALL_K3S_EXEC="--disable traefik --write-kubeconfig-mode 644" sh -

mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown "$USER:$USER" ~/.kube/config
export KUBECONFIG=~/.kube/config
echo 'export KUBECONFIG=~/.kube/config' >> ~/.bashrc

kubectl get nodes -o wide
```

## 2. Install Helm and chart repos

```bash
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update
```

## 3. Clone or update source

```bash
sudo mkdir -p /opt
sudo chown "$USER:$USER" /opt
cd /opt
[ -d NexusChat/.git ] || git clone https://github.com/Tuananh165-art/NexusChat.git
cd /opt/NexusChat
git pull --ff-only
```

## 4. Create namespaces

```bash
for ns in ingress-nginx nexuschat-lab redis kafka cassandra minio postgres monitoring argocd redis-ui; do
  kubectl create namespace "$ns" --dry-run=client -o yaml | kubectl apply -f -
done
```

## 5. Install ingress-nginx

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
kubectl -n ingress-nginx get svc,pods -o wide
```

## 6. Install lightweight dependencies

Set lab credentials in shell. These are examples; use stronger values when exposed outside a private lab.

```bash
export REDIS_PASSWORD='lab-redis-pass'
export CASSANDRA_PASSWORD='lab-cassandra-pass'
export MINIO_ACCESS_KEY='labminio'
export MINIO_SECRET_KEY='lab-minio-secret'
export AI_POSTGRES_PASSWORD='lab-postgres-pass'
```

### 6.1 Redis Cluster

Current Go code uses Redis Cluster client, so use Redis Cluster, not standalone Redis.

```bash
helm uninstall redis -n redis || true
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
  --set persistence.size=1Gi \
  --set redis.resources.requests.cpu=25m \
  --set redis.resources.requests.memory=96Mi \
  --set redis.resources.limits.cpu=200m \
  --set redis.resources.limits.memory=192Mi
kubectl -n redis get pods,svc -o wide
```

Expected Helm service names in `values-lab-k3s.yaml` point at `redis-redis-cluster-0/1/2.redis-redis-cluster-headless.redis.svc.cluster.local:6379`.

### 6.2 Kafka

Preferred lightweight official-image manifest if Bitnami image pulls are unreliable:

```bash
helm uninstall kafka -n kafka || true
cat <<'EOF' | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: kafka
  namespace: kafka
spec:
  selector:
    app: kafka
  ports:
    - name: client
      port: 9092
      targetPort: 9092
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: kafka
  namespace: kafka
spec:
  serviceName: kafka
  replicas: 1
  selector:
    matchLabels:
      app: kafka
  template:
    metadata:
      labels:
        app: kafka
    spec:
      containers:
        - name: kafka
          image: confluentinc/cp-kafka:7.6.0
          ports:
            - containerPort: 9092
              name: client
            - containerPort: 9093
              name: controller
          env:
            - name: KAFKA_PROCESS_ROLES
              value: controller,broker
            - name: KAFKA_NODE_ID
              value: "1"
            - name: KAFKA_CONTROLLER_QUORUM_VOTERS
              value: "1@localhost:9093"
            - name: KAFKA_LISTENERS
              value: PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093
            - name: KAFKA_LISTENER_SECURITY_PROTOCOL_MAP
              value: PLAINTEXT:PLAINTEXT,CONTROLLER:PLAINTEXT
            - name: KAFKA_INTER_BROKER_LISTENER_NAME
              value: PLAINTEXT
            - name: KAFKA_CONTROLLER_LISTENER_NAMES
              value: CONTROLLER
            - name: KAFKA_ADVERTISED_LISTENERS
              value: PLAINTEXT://kafka.kafka.svc.cluster.local:9092
            - name: CLUSTER_ID
              value: ciWo7IWazngRchmPES6q5A==
            - name: KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR
              value: "1"
            - name: KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR
              value: "1"
            - name: KAFKA_TRANSACTION_STATE_LOG_MIN_ISR
              value: "1"
            - name: KAFKA_AUTO_CREATE_TOPICS_ENABLE
              value: "true"
            - name: KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS
              value: "0"
            - name: KAFKA_HEAP_OPTS
              value: "-Xms256m -Xmx512m"
          resources:
            requests:
              cpu: 100m
              memory: 512Mi
            limits:
              cpu: 500m
              memory: 900Mi
          volumeMounts:
            - name: data
              mountPath: /var/lib/kafka/data
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 5Gi
EOF
kubectl -n kafka rollout status statefulset/kafka --timeout=300s
```

### 6.3 Cassandra

```bash
helm uninstall cassandra -n cassandra || true
cat <<'EOF' | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: cassandra
  namespace: cassandra
spec:
  selector:
    app: cassandra
  ports:
    - name: cql
      port: 9042
      targetPort: 9042
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: cassandra
  namespace: cassandra
spec:
  serviceName: cassandra
  replicas: 1
  selector:
    matchLabels:
      app: cassandra
  template:
    metadata:
      labels:
        app: cassandra
    spec:
      containers:
        - name: cassandra
          image: cassandra:4.0
          ports:
            - containerPort: 9042
              name: cql
          env:
            - name: CASSANDRA_CLUSTER_NAME
              value: NexusChat
            - name: CASSANDRA_SEEDS
              value: cassandra
            - name: MAX_HEAP_SIZE
              value: 512M
            - name: HEAP_NEWSIZE
              value: 128M
          resources:
            requests:
              cpu: 100m
              memory: 768Mi
            limits:
              cpu: 700m
              memory: 1200Mi
          volumeMounts:
            - name: data
              mountPath: /var/lib/cassandra
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 8Gi
EOF
kubectl -n cassandra rollout status statefulset/cassandra --timeout=600s
```

Apply schema:

```bash
kubectl delete pod cassandra-schema-client -n cassandra --ignore-not-found
kubectl run cassandra-schema-client -n cassandra --restart=Never --image=python:3.12-slim --env CASSANDRA_HOST=cassandra.cassandra.svc.cluster.local --command -- sleep 3600
kubectl wait --for=condition=Ready pod/cassandra-schema-client -n cassandra --timeout=180s
kubectl cp cassandra/init.cql cassandra/cassandra-schema-client:/tmp/init.cql
kubectl exec -n cassandra cassandra-schema-client -- sh -lc "pip install cassandra-driver && python -c \"from cassandra.cluster import Cluster; import os, pathlib; schema=pathlib.Path('/tmp/init.cql').read_text(); cluster=Cluster([os.environ['CASSANDRA_HOST']], port=9042); session=cluster.connect(); [session.execute(stmt.strip() + ';') for stmt in schema.split(';') if stmt.strip()]; cluster.shutdown(); print('schema applied')\""
kubectl delete pod cassandra-schema-client -n cassandra --ignore-not-found
```

### 6.4 MinIO and bucket

```bash
helm uninstall minio -n minio || true
kubectl delete deployment minio -n minio --ignore-not-found
kubectl delete svc minio -n minio --ignore-not-found

kubectl create deployment minio -n minio --image=minio/minio:RELEASE.2023-07-11T21-29-34Z --port=9000 -- minio server /data --console-address :9001
kubectl set env deployment/minio -n minio MINIO_ROOT_USER="$MINIO_ACCESS_KEY" MINIO_ROOT_PASSWORD="$MINIO_SECRET_KEY"
kubectl set resources deployment/minio -n minio --requests=cpu=50m,memory=128Mi --limits=cpu=300m,memory=384Mi
kubectl expose deployment minio -n minio --name=minio --port=9000 --target-port=9000
kubectl rollout status deployment/minio -n minio --timeout=180s

kubectl run minio-mc -n minio --rm -i --restart=Never --image=minio/mc:RELEASE.2023-07-11T23-30-44Z --command -- /bin/sh -c "mc alias set local http://minio.minio.svc.cluster.local:9000 $MINIO_ACCESS_KEY $MINIO_SECRET_KEY && mc mb --ignore-existing local/myfilebucket && mc anonymous set private local/myfilebucket"
```

If you need console UI, expose/create a console service and use `deployments/platform/ingresses.yaml` only after matching service names.

### 6.5 PostgreSQL for AI service

```bash
helm upgrade --install ai-postgres bitnami/postgresql \
  --namespace postgres \
  --set auth.database=nexuschat_ai \
  --set auth.username=nexuschat_ai \
  --set auth.password="$AI_POSTGRES_PASSWORD" \
  --set primary.persistence.enabled=true \
  --set primary.persistence.size=5Gi \
  --set primary.resources.requests.cpu=50m \
  --set primary.resources.requests.memory=128Mi \
  --set primary.resources.limits.cpu=300m \
  --set primary.resources.limits.memory=384Mi
kubectl -n postgres rollout status statefulset/ai-postgres-postgresql --timeout=300s
```

Check all dependencies:

```bash
kubectl get pods -n redis
kubectl get pods -n kafka
kubectl get pods -n cassandra
kubectl get pods -n minio
kubectl get pods -n postgres
```

## 7. Create runtime secret

Use real OAuth/AI values if testing those features. Placeholders allow pods to start but will not make OAuth/AI work.

```bash
export GOOGLE_CLIENT_ID='replace-me'
export GOOGLE_CLIENT_SECRET='replace-me'
export AI_ENDPOINT='https://replace-me-openai-compatible/v1'
export AI_API_KEY='replace-me'
export AI_MODEL='replace-me'

kubectl create secret generic nexuschat-runtime \
  --namespace nexuschat-lab \
  --from-literal=CHAT_JWT_SECRET='lab-jwt-secret-change-me' \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --from-literal=CASSANDRA_USER='admin' \
  --from-literal=CASSANDRA_PASSWORD="$CASSANDRA_PASSWORD" \
  --from-literal=UPLOADER_S3_ACCESSKEY="$MINIO_ACCESS_KEY" \
  --from-literal=UPLOADER_S3_SECRETKEY="$MINIO_SECRET_KEY" \
  --from-literal=USER_OAUTH_GOOGLE_CLIENTID="$GOOGLE_CLIENT_ID" \
  --from-literal=USER_OAUTH_GOOGLE_CLIENTSECRET="$GOOGLE_CLIENT_SECRET" \
  --from-literal=DATABASE_URL="postgresql+asyncpg://nexuschat_ai:$AI_POSTGRES_PASSWORD@ai-postgres-postgresql.postgres.svc.cluster.local:5432/nexuschat_ai" \
  --from-literal=AI_ENDPOINT="$AI_ENDPOINT" \
  --from-literal=AI_API_KEY="$AI_API_KEY" \
  --from-literal=AI_MODEL="$AI_MODEL" \
  --from-literal=AI_POSTGRES_PASSWORD="$AI_POSTGRES_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -
```

## 8. Deploy manually with existing Docker Hub images

If images already exist:

```bash
export TAG='<existing-sha-or-tag>'
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

If your `TAG` does not have proxy variant images, either push them first or remove/replace the two `services.*.image.fullname` overrides.

## 9. GitHub Actions deploy pipeline

Workflow file: `.github/workflows/devsecops-platform.yml`.

Triggers:

- PR: validation only.
- Push `main`: validation, image build/scan/SBOM/sign, proxy variant build, direct Helm deploy to `nexuschat-lab`.
- Push `kafka` or tag `v*`: validation + image build/scan/SBOM/sign, no lab deploy unless branch is `main`.
- Manual: `workflow_dispatch`.

Required GitHub secrets:

```text
DOCKER_USERNAME
DOCKER_PASSWORD
KUBE_CONFIG_B64
```

`KUBE_CONFIG_B64` may be empty only if the self-hosted runner already has a working kubeconfig. Current job runs on:

```text
[self-hosted, linux, x64, k3s-lab]
```

To trigger deploy:

```bash
git status --short
git add <reviewed-files>
git commit -m "your message"
git push origin main
```

## 10. Verify rollout

```bash
kubectl -n nexuschat-lab get pods -o wide
kubectl -n nexuschat-lab get svc
kubectl -n nexuschat-lab get ingress
kubectl -n nexuschat-lab get deploy -o jsonpath='{range .items[*]}{.metadata.name}{" => "}{range .spec.template.spec.containers[*]}{.image}{" "}{end}{"\n"}{end}'

for deploy in web chat match user uploader forwarder ai-service; do
  kubectl -n nexuschat-lab rollout status deployment/$deploy --timeout=600s
done

curl -I http://192.168.109.131
curl -i http://192.168.109.131/api/ai/health
```

Check logs:

```bash
kubectl -n nexuschat-lab logs deploy/web --tail=100
kubectl -n nexuschat-lab logs deploy/chat --tail=100
kubectl -n nexuschat-lab logs deploy/uploader --tail=100
kubectl -n nexuschat-lab logs deploy/ai-service --tail=100
```

## 11. Common failures

### Redis standalone error

Symptom:

```text
ERR This instance has cluster support disabled
```

Fix: use Redis Cluster and ensure `REDIS_ADDRS` points at cluster node/headless endpoints.

### Uploader `NoSuchBucket`

Fix: create MinIO/S3 bucket `myfilebucket` and confirm uploader env `UPLOADER_S3_BUCKET` matches.

### AI features fail with provider DNS/placeholder errors

Inspect `nexuschat-runtime` keys `AI_ENDPOINT`, `AI_API_KEY`, `AI_MODEL`, `DATABASE_URL`. Restart only `ai-service` after secret update:

```bash
kubectl -n nexuschat-lab rollout restart deployment/ai-service
kubectl -n nexuschat-lab rollout status deployment/ai-service --timeout=300s
```

### Ingress returns 503

Check service endpoints and ingress class:

```bash
kubectl -n nexuschat-lab describe ingress
kubectl -n nexuschat-lab get endpoints
kubectl -n ingress-nginx logs deploy/ingress-nginx-controller --tail=100
```

## 12. Resource monitoring

```bash
kubectl top nodes || true
kubectl top pods -A || true
free -h
df -h
```

Install metrics-server if needed:

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl patch deployment metrics-server -n kube-system --type=json -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
```

## 13. Rollback and cleanup

Rollback:

```bash
helm history nexuschat -n nexuschat-lab
helm rollback nexuschat <REVISION> -n nexuschat-lab
for deploy in web chat match user uploader forwarder ai-service; do kubectl -n nexuschat-lab rollout status deployment/$deploy --timeout=600s; done
```

Cleanup app:

```bash
helm uninstall nexuschat -n nexuschat-lab
```

Cleanup dependencies:

```bash
helm uninstall redis -n redis || true
kubectl delete statefulset,svc -n kafka -l app=kafka || true
kubectl delete statefulset,svc -n cassandra -l app=cassandra || true
kubectl delete deployment,svc -n minio -l app=minio || true
helm uninstall ai-postgres -n postgres || true
```

Remove K3s:

```bash
sudo /usr/local/bin/k3s-uninstall.sh
```

## 14. Final lab checklist

- [ ] K3s node Ready.
- [ ] ingress-nginx pod Ready.
- [ ] Redis Cluster, Kafka, Cassandra, MinIO, PostgreSQL Ready.
- [ ] Cassandra schema applied.
- [ ] MinIO bucket `myfilebucket` exists.
- [ ] Secret `nexuschat-runtime` exists in `nexuschat-lab`.
- [ ] Docker Hub images for selected tag exist.
- [ ] Helm release `nexuschat` deployed with `values-lab-k3s.yaml`.
- [ ] All 7 deployments rolled out.
- [ ] `curl http://192.168.109.131` responds.
- [ ] `curl http://192.168.109.131/api/ai/health` responds.
