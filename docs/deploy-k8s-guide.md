# NexusChat Lab Deployment Guide for a 4GB RAM / 50GB Disk Server

This document is intended for a small lab or personal demo with no real users. The goal is to run NexusChat with the minimum viable footprint on a low-resource server. This is not a production configuration.

## Quick Summary

A `4GB RAM / 50GB disk` server should not run the full DevSecOps stack. Do not install these components on this server:

- ELK / ECK / Elasticsearch / Kibana / Filebeat.
- Consul service mesh.
- Full kube-prometheus-stack.
- HA configurations for Kafka, Cassandra, Redis, Postgres, or MinIO.
- HA ArgoCD.

Recommended components:

- Single-node K3s.
- Lightweight Nginx Ingress.
- Minimal ArgoCD, or skip ArgoCD and use `helm upgrade --install` directly.
- NexusChat with `values-lab-4gb.yaml`.
- Single-node Redis.
- Single-node Kafka.
- Single-node Cassandra with reduced resources.
- Single-node Postgres.
- Single-node MinIO.

If the server runs out of RAM, prefer local Docker Compose or move Kafka/Cassandra/Postgres/MinIO to managed or external services.

## Lab Configuration File

Dedicated Helm values for a 4GB server:

```text
deployments/helm/nexuschat/values-lab-4gb.yaml
```

This file:

- Reduces all service replicas to `1`.
- Disables autoscaling.
- Disables ServiceMonitor.
- Disables NetworkPolicy to avoid issues when the lab CNI is not ready.
- Disables Consul annotations.
- Disables the tracing endpoint.
- Reduces CPU/RAM requests and limits.
- Allows direct access by server IP without requiring a domain.
- Disables the default HTTPS redirect.

## 1. Install Lightweight K3s

```bash
curl -sfL https://get.k3s.io | sudo INSTALL_K3S_EXEC="--disable traefik --write-kubeconfig-mode 644" sh -

mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown "$USER:$USER" ~/.kube/config
export KUBECONFIG=~/.kube/config
echo 'export KUBECONFIG=~/.kube/config' >> ~/.bashrc

kubectl get nodes -o wide
```

## 2. Install Helm

```bash
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo add argo https://argoproj.github.io/argo-helm
helm repo update
```

## 3. Clone the Repository

```bash
sudo mkdir -p /opt
sudo chown "$USER:$USER" /opt
cd /opt
git clone https://github.com/Tuananh165-art/NexusChat.git
cd /opt/NexusChat
```

If the repository already exists:

```bash
cd /opt/NexusChat
git pull --ff-only
```

## 4. Create Namespaces

```bash
kubectl create namespace ingress-nginx --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace nexuschat-lab --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace redis --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace kafka --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace cassandra --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace minio --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace postgres --dry-run=client -o yaml | kubectl apply -f -
```

## 5. Install Lightweight Nginx Ingress

```bash
helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx \
  --set controller.replicaCount=1 \
  --set controller.metrics.enabled=false \
  --set controller.resources.requests.cpu=50m \
  --set controller.resources.requests.memory=96Mi \
  --set controller.resources.limits.cpu=300m \
  --set controller.resources.limits.memory=256Mi
```

Check the installation:

```bash
kubectl get pods -n ingress-nginx
kubectl get svc -n ingress-nginx
```

## 6. Install Lightweight Dependencies

These are example passwords for a lab. If the server is exposed to the public internet, use stronger passwords.

```bash
export REDIS_PASSWORD='lab-redis-pass'
export CASSANDRA_PASSWORD='lab-cassandra-pass'
export MINIO_ACCESS_KEY='labminio'
export MINIO_SECRET_KEY='lab-minio-secret'
export AI_POSTGRES_PASSWORD='lab-postgres-pass'
```

### Redis

The NexusChat Go backend currently uses a Redis Cluster client. For that reason, the lab should not use standalone `bitnami/redis`; with standalone Redis, the Go pods will fail with:

```text
ERR This instance has cluster support disabled
```

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
```

Check Redis Cluster:

```bash
kubectl get pods -n redis
kubectl get svc -n redis
```

### Kafka

Kafka will likely be the heaviest component in the lab. If the server is short on RAM, move Kafka outside the server or disable Kafka-dependent flows during testing.

```bash
helm upgrade --install kafka bitnami/kafka \
  --namespace kafka \
  --set kraft.enabled=true \
  --set controller.replicaCount=1 \
  --set broker.replicaCount=0 \
  --set listeners.client.protocol=PLAINTEXT \
  --set persistence.enabled=true \
  --set persistence.size=5Gi \
  --set controller.resources.requests.cpu=100m \
  --set controller.resources.requests.memory=384Mi \
  --set controller.resources.limits.cpu=500m \
  --set controller.resources.limits.memory=768Mi
```

If Kafka gets stuck in `Init:ImagePullBackOff`, check the image and events:

```bash
kubectl describe pod kafka-controller-0 -n kafka | sed -n '/Events:/,$p'
kubectl get pod kafka-controller-0 -n kafka -o jsonpath='{range .spec.initContainers[*]}{.name}{" => "}{.image}{"\n"}{end}{range .spec.containers[*]}{.name}{" => "}{.image}{"\n"}{end}'
```

If the events show that `bitnami/kafka` or a Bitnami init image cannot be pulled, a more stable lab option is to remove the Bitnami Kafka chart and use a minimal Kafka manifest with the `confluentinc/cp-kafka:7.6.0` image:

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
            - name: KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS
              value: "0"
            - name: KAFKA_AUTO_CREATE_TOPICS_ENABLE
              value: "true"
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
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 5Gi
EOF
```

### Cassandra

Cassandra is also very heavy for a 4GB server. The configuration below is only for a small lab.

```bash
helm upgrade --install cassandra bitnami/cassandra \
  --namespace cassandra \
  --set dbUser.user=admin \
  --set dbUser.password="$CASSANDRA_PASSWORD" \
  --set cluster.name=NexusChat \
  --set replicaCount=1 \
  --set persistence.enabled=true \
  --set persistence.size=8Gi \
  --set jvm.maxHeapSize=512m \
  --set jvm.newHeapSize=128m \
  --set resources.requests.cpu=100m \
  --set resources.requests.memory=768Mi \
  --set resources.limits.cpu=700m \
  --set resources.limits.memory=1200Mi
```

If Cassandra gets stuck in `ImagePullBackOff`, check the image and events:

```bash
kubectl describe pod cassandra-0 -n cassandra | sed -n '/Events:/,$p'
kubectl get pod cassandra-0 -n cassandra -o jsonpath='{range .spec.initContainers[*]}{.name}{" => "}{.image}{"\n"}{end}{range .spec.containers[*]}{.name}{" => "}{.image}{"\n"}{end}'
```

If the events show that `bitnami/cassandra` cannot be pulled, a more stable lab option is to remove the Bitnami Cassandra chart and use the official `cassandra:4.0` image:

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
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 8Gi
EOF
```

Apply the schema after Cassandra is `Running` and `Ready`:

```bash
kubectl delete pod cassandra-schema-client -n cassandra --ignore-not-found
kubectl run cassandra-schema-client \
  -n cassandra \
  --restart=Never \
  --image=python:3.12-slim \
  --env CASSANDRA_HOST=cassandra.cassandra.svc.cluster.local \
  --command -- sleep 3600

kubectl wait --for=condition=Ready pod/cassandra-schema-client -n cassandra --timeout=120s
kubectl cp cassandra/init.cql cassandra/cassandra-schema-client:/tmp/init.cql

kubectl exec -n cassandra cassandra-schema-client -- sh -lc "pip install cassandra-driver && python -c \"from cassandra.cluster import Cluster; import os, pathlib; schema=pathlib.Path('/tmp/init.cql').read_text(); cluster=Cluster([os.environ['CASSANDRA_HOST']], port=9042); session=cluster.connect(); [session.execute(stmt.strip() + ';') for stmt in schema.split(';') if stmt.strip()]; cluster.shutdown(); print('schema applied')\""

kubectl delete pod cassandra-schema-client -n cassandra --ignore-not-found
```

### MinIO

```bash
helm upgrade --install minio bitnami/minio \
  --namespace minio \
  --set auth.rootUser="$MINIO_ACCESS_KEY" \
  --set auth.rootPassword="$MINIO_SECRET_KEY" \
  --set defaultBuckets=myfilebucket \
  --set persistence.enabled=true \
  --set persistence.size=5Gi \
  --set resources.requests.cpu=50m \
  --set resources.requests.memory=128Mi \
  --set resources.limits.cpu=300m \
  --set resources.limits.memory=384Mi
```

If Bitnami MinIO has image pull errors or you need a lighter option, use the official MinIO image:

```bash
helm uninstall minio -n minio || true
kubectl delete deployment minio -n minio --ignore-not-found
kubectl delete deployment minio-console -n minio --ignore-not-found
kubectl delete svc minio -n minio --ignore-not-found

kubectl create deployment minio \
  -n minio \
  --image=minio/minio:RELEASE.2023-07-11T21-29-34Z \
  --port=9000 \
  -- minio server /data --console-address :9001

kubectl set env deployment/minio -n minio \
  MINIO_ROOT_USER="$MINIO_ACCESS_KEY" \
  MINIO_ROOT_PASSWORD="$MINIO_SECRET_KEY"

kubectl set resources deployment/minio -n minio \
  --requests=cpu=50m,memory=128Mi \
  --limits=cpu=300m,memory=384Mi

kubectl expose deployment minio \
  -n minio \
  --name=minio \
  --port=9000 \
  --target-port=9000

kubectl rollout status deployment/minio -n minio --timeout=120s
```

Create the bucket:

```bash
kubectl run minio-mc \
  -n minio \
  --rm -i --restart=Never \
  --image=minio/mc:RELEASE.2023-07-11T23-30-44Z \
  --command -- /bin/sh -c "mc alias set local http://minio.minio.svc.cluster.local:9000 $MINIO_ACCESS_KEY $MINIO_SECRET_KEY && mc mb --ignore-existing local/myfilebucket && mc anonymous set private local/myfilebucket"
```

### Postgres

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
```

Check the dependencies:

```bash
kubectl get pods -n redis
kubectl get pods -n kafka
kubectl get pods -n cassandra
kubectl get pods -n minio
kubectl get pods -n postgres
```

## 7. Build and Push Images

Building with GitHub Actions is recommended. Building on a 4GB server can be very slow or run out of RAM.

### 7.1 Set Up GitHub Actions CI/CD for the Lab

The repository already includes this workflow:

```text
.github/workflows/devsecops-platform.yml
```

The workflow performs these main tasks:

- Tests the Go backend.
- Builds and lints the frontend.
- Tests and lints the AI service.
- Validates the Helm chart production profile.
- Validates the Helm chart 4GB lab profile.
- Runs secret scanning with Gitleaks.
- Runs CodeQL scanning.
- Runs Trivy filesystem scanning.
- Builds and pushes 3 images to GHCR:
  - `ghcr.io/tuananh165-art/nexuschat/api:<tag>`
  - `ghcr.io/tuananh165-art/nexuschat/web:<tag>`
  - `ghcr.io/tuananh165-art/nexuschat/ai-service:<tag>`
- Scans images.
- Creates an SBOM.
- Signs images with Cosign keyless signing.

### 7.2 Enable GHCR Permissions for GitHub Actions

In the GitHub repository:

1. Go to `Settings`.
2. Go to `Actions` -> `General`.
3. Under `Workflow permissions`, select `Read and write permissions`.
4. Enable `Allow GitHub Actions to create and approve pull requests` if your process needs bots to create PRs.
5. Save.

The workflow already declares:

```yaml
permissions:
  contents: read
  packages: write
  security-events: write
  id-token: write
  actions: read
```

For public GHCR packages, Kubernetes can pull the images without an image pull secret. For private GHCR packages, create `ghcr-pull-secret` in the `nexuschat-lab` namespace.

### 7.3 Run the Pipeline Manually

The workflow supports `workflow_dispatch`, so it can be run manually:

1. Go to the `Actions` tab.
2. Select `DevSecOps Platform Pipeline`.
3. Click `Run workflow`.
4. Select the `main` branch.
5. Click `Run workflow`.

Or just push to `main`:

```bash
git add .
git commit -m "Configure NexusChat lab deployment"
git push origin main
```

After the workflow succeeds, the images will be tagged with the full commit SHA.

Get the commit SHA on the server:

```bash
cd /opt/NexusChat
git pull --ff-only
export IMAGE_TAG=$(git rev-parse HEAD)
```

If you build from a release tag, for example `v0.1.0`, use:

```bash
export IMAGE_TAG='v0.1.0'
```

### 7.4 Check Images on GHCR

Check in GitHub:

1. Go to the repository or organization/user packages.
2. Open Packages.
3. Confirm that these packages exist:
   - `nexuschat/api`
   - `nexuschat/web`
   - `nexuschat/ai-service`

If the packages are private, create a GitHub token with `read:packages`, then create the pull secret:

```bash
kubectl create secret docker-registry ghcr-pull-secret \
  --namespace nexuschat-lab \
  --docker-server=ghcr.io \
  --docker-username=<github-username> \
  --docker-password=<github-token-read-packages> \
  --docker-email=<email> \
  --dry-run=client -o yaml | kubectl apply -f -
```

When deploying with Helm, add:

```bash
--set global.imagePullSecrets[0].name=ghcr-pull-secret
```

Example:

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-4gb.yaml \
  --set imageDefaults.tag="$IMAGE_TAG" \
  --set global.imagePullSecrets[0].name=ghcr-pull-secret
```

### 7.5 If You Do Not Use GitHub Actions

If the images already exist on GHCR, choose a tag:

```bash
export IMAGE_TAG='<image-tag>'
```

If you must build manually:

```bash
export REGISTRY=ghcr.io/tuananh165-art/nexuschat
export IMAGE_TAG=$(git rev-parse --short HEAD)

docker login ghcr.io

docker build -f build/Dockerfile.api --build-arg VERSION="$IMAGE_TAG" -t "$REGISTRY/api:$IMAGE_TAG" .
docker build -f build/Dockerfile.web --build-arg VERSION="$IMAGE_TAG" -t "$REGISTRY/web:$IMAGE_TAG" .
docker build -f ai-service/Dockerfile -t "$REGISTRY/ai-service:$IMAGE_TAG" ai-service

docker push "$REGISTRY/api:$IMAGE_TAG"
docker push "$REGISTRY/web:$IMAGE_TAG"
docker push "$REGISTRY/ai-service:$IMAGE_TAG"
```

## 8. Create the NexusChat Secret

If you are not using real OAuth or real AI credentials yet, placeholders are still needed so the pods have complete environment variables.

```bash
kubectl create secret generic nexuschat-runtime \
  --namespace nexuschat-lab \
  --from-literal=CHAT_JWT_SECRET='lab-jwt-secret-change-me' \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --from-literal=CASSANDRA_USER='admin' \
  --from-literal=CASSANDRA_PASSWORD="$CASSANDRA_PASSWORD" \
  --from-literal=UPLOADER_S3_ACCESSKEY="$MINIO_ACCESS_KEY" \
  --from-literal=UPLOADER_S3_SECRETKEY="$MINIO_SECRET_KEY" \
  --from-literal=USER_OAUTH_GOOGLE_CLIENTID='lab-google-client-id' \
  --from-literal=USER_OAUTH_GOOGLE_CLIENTSECRET='lab-google-client-secret' \
  --from-literal=AI_POSTGRES_PASSWORD="$AI_POSTGRES_PASSWORD" \
  --from-literal=DATABASE_URL="postgresql+asyncpg://nexuschat_ai:$AI_POSTGRES_PASSWORD@ai-postgres-postgresql.postgres.svc.cluster.local:5432/nexuschat_ai" \
  --from-literal=AI_ENDPOINT='http://placeholder-ai-endpoint/v1' \
  --from-literal=AI_API_KEY='placeholder-ai-key' \
  --from-literal=AI_MODEL='placeholder-model' \
  --dry-run=client -o yaml | kubectl apply -f -
```

## 9. Deploy NexusChat Lab Directly with Helm

This is the lightest approach for a 4GB server. ArgoCD is not required.

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-4gb.yaml \
  --set imageDefaults.tag="$IMAGE_TAG"
```

Check the deployment:

```bash
kubectl get pods -n nexuschat-lab -o wide
kubectl get svc -n nexuschat-lab
kubectl get ingress -n nexuschat-lab
```

## 10. Access by Server IP

Get the server IP:

```bash
hostname -I
```

With `values-lab-4gb.yaml`, `global.domain` is empty, so the Ingress will not bind a host. You can call it directly by IP:

```bash
curl -I http://<server-ip>
curl -I http://<server-ip>/api/ai/health
```

If you still want to use the name `nexuschat.local`, add it to the hosts file on your client machine:

```text
<server-ip> nexuschat.local
```

Then override the domain during deployment:

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-4gb.yaml \
  --set imageDefaults.tag="$IMAGE_TAG" \
  --set global.domain=nexuschat.local \
  --set services.user.env.USER_AUTH_COOKIE_DOMAIN=nexuschat.local \
  --set services.user.env.USER_OAUTH_COOKIE_DOMAIN=nexuschat.local \
  --set services.uploader.env.UPLOADER_S3_PUBLICENDPOINT=http://nexuschat.local
```

## 11. Monitor Resource Usage

```bash
kubectl top nodes
kubectl top pods -A
df -h
free -h
```

If `kubectl top` does not work yet, install metrics-server:

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl patch deployment metrics-server -n kube-system --type=json \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
```

## 12. If the Server Is Overloaded

Use this priority order to reduce load:

1. Disable the AI service if you are not testing AI yet:

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-4gb.yaml \
  --set imageDefaults.tag="$IMAGE_TAG" \
  --set services.ai-service.enabled=false
```

2. Move Postgres and MinIO outside the server.
3. Move Kafka outside the server.
4. Move Cassandra outside the server.
5. Switch back to Docker Compose or upgrade the server to at least 8GB RAM.

## 13. Do Not Install These on a 4GB Server

Do not apply these files if the server only has 4GB RAM:

```text
deployments/gitops/applications/platform-apps.yaml
deployments/platform/logging/eck-stack.yaml
deployments/platform/consul/values.yaml
deployments/platform/monitoring/kube-prometheus-stack-values.yaml
```

If you want to use ArgoCD for the lab, install only single-replica ArgoCD and deploy the app. Do not install all platform apps.

## 14. Cleanup When Needed

Remove the app:

```bash
helm uninstall nexuschat -n nexuschat-lab
```

Remove dependencies:

```bash
helm uninstall redis -n redis
helm uninstall kafka -n kafka
helm uninstall cassandra -n cassandra
helm uninstall minio -n minio
helm uninstall ai-postgres -n postgres
```

Remove K3s:

```bash
sudo /usr/local/bin/k3s-uninstall.sh
```

## Lab Checklist

- K3s is ready.
- Helm is ready.
- Nginx Ingress is ready.
- Redis, Kafka, Cassandra, MinIO, and Postgres are ready.
- Images exist in the registry.
- Secret `nexuschat-runtime` exists in `nexuschat-lab`.
- NexusChat is deployed with Helm using `values-lab-4gb.yaml`.
- `curl http://<server-ip>` returns a response.
