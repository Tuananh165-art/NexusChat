# Huong Dan Chay NexusChat Lab Tren Server 4GB RAM / 50GB Disk

Tai lieu nay dung cho lab nho, demo ca nhan, khong co user that. Muc tieu la chay duoc NexusChat o muc toi thieu tren server yeu. Day khong phai cau hinh production.

## Ket Luan Nhanh

Server `4GB RAM / 50GB disk` khong nen apply toan bo stack DevSecOps. Khong cai cac thanh phan sau tren server nay:

- ELK / ECK / Elasticsearch / Kibana / Filebeat.
- Consul service mesh.
- kube-prometheus-stack day du.
- Kafka, Cassandra, Redis, Postgres, MinIO cau hinh HA.
- ArgoCD HA.

Nen chay:

- K3s single-node.
- Nginx Ingress ban nhe.
- ArgoCD ban toi thieu hoac bo ArgoCD va dung `helm upgrade --install` truc tiep.
- NexusChat voi `values-lab-4gb.yaml`.
- Redis single-node.
- Kafka single-node.
- Cassandra single-node voi resource nho.
- Postgres single-node.
- MinIO single-node.

Neu server bi thieu RAM, uu tien dung Docker Compose local hoac dua Kafka/Cassandra/Postgres/MinIO ra managed/external services.

## File Cau Hinh Lab

Helm values rieng cho server 4GB:

```text
deployments/helm/nexuschat/values-lab-4gb.yaml
```

File nay da:

- Giam tat ca service replica ve `1`.
- Tat autoscaling.
- Tat ServiceMonitor.
- Tat NetworkPolicy de tranh loi khi CNI lab chua san sang.
- Tat Consul annotations.
- Tat tracing endpoint.
- Giam CPU/RAM requests va limits.
- Co the truy cap truc tiep bang IP server, khong can domain.
- Tat redirect HTTPS mac dinh.

## 1. Cai K3s Nhe

```bash
curl -sfL https://get.k3s.io | sudo INSTALL_K3S_EXEC="--disable traefik --write-kubeconfig-mode 644" sh -

mkdir -p ~/.kube
sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config
sudo chown "$USER:$USER" ~/.kube/config
export KUBECONFIG=~/.kube/config
echo 'export KUBECONFIG=~/.kube/config' >> ~/.bashrc

kubectl get nodes -o wide
```

## 2. Cai Helm

```bash
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo add argo https://argoproj.github.io/argo-helm
helm repo update
```

## 3. Clone Repo

```bash
sudo mkdir -p /opt
sudo chown "$USER:$USER" /opt
cd /opt
git clone https://github.com/Tuananh165-art/NexusChat.git
cd /opt/NexusChat
```

Neu repo da co:

```bash
cd /opt/NexusChat
git pull --ff-only
```

## 4. Tao Namespace

```bash
kubectl create namespace ingress-nginx --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace nexuschat-lab --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace redis --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace kafka --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace cassandra --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace minio --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace postgres --dry-run=client -o yaml | kubectl apply -f -
```

## 5. Cai Nginx Ingress Nhe

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

Kiem tra:

```bash
kubectl get pods -n ingress-nginx
kubectl get svc -n ingress-nginx
```

## 6. Cai Dependencies Ban Nhe

Dung password vi du trong lab. Neu server public internet, hay doi password manh hon.

```bash
export REDIS_PASSWORD='lab-redis-pass'
export CASSANDRA_PASSWORD='lab-cassandra-pass'
export MINIO_ACCESS_KEY='labminio'
export MINIO_SECRET_KEY='lab-minio-secret'
export AI_POSTGRES_PASSWORD='lab-postgres-pass'
```

### Redis

```bash
helm upgrade --install redis bitnami/redis \
  --namespace redis \
  --set auth.enabled=true \
  --set auth.password="$REDIS_PASSWORD" \
  --set architecture=standalone \
  --set master.persistence.enabled=true \
  --set master.persistence.size=2Gi \
  --set master.resources.requests.cpu=25m \
  --set master.resources.requests.memory=96Mi \
  --set master.resources.limits.cpu=200m \
  --set master.resources.limits.memory=192Mi
```

### Kafka

Kafka kha nang se la thanh phan nang nhat trong lab. Neu server thieu RAM, hay dua Kafka ra ngoai hoac tat cac flow phu thuoc Kafka khi test.

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

### Cassandra

Cassandra cung rat nang voi server 4GB. Cau hinh duoi day chi de lab nho.

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

Apply schema:

```bash
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=cassandra -n cassandra --timeout=15m

kubectl create configmap nexuschat-cassandra-schema \
  --namespace cassandra \
  --from-file=init.cql=cassandra/init.cql \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl run cassandra-schema-init \
  --namespace cassandra \
  --rm -i --restart=Never \
  --image=cassandra:4.0 \
  --overrides='{"spec":{"volumes":[{"name":"schema","configMap":{"name":"nexuschat-cassandra-schema"}}],"containers":[{"name":"cassandra-schema-init","image":"cassandra:4.0","command":["bash","-lc","cqlsh cassandra.cassandra.svc.cluster.local 9042 -u admin -p '"'"$CASSANDRA_PASSWORD"'"' -f /schema/init.cql"],"volumeMounts":[{"name":"schema","mountPath":"/schema"}]}]}}'
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

Kiem tra dependencies:

```bash
kubectl get pods -n redis
kubectl get pods -n kafka
kubectl get pods -n cassandra
kubectl get pods -n minio
kubectl get pods -n postgres
```

## 7. Build Va Push Images

Khuyen nghi build bang GitHub Actions. Neu build tren server 4GB, co the rat cham hoac het RAM.

### 7.1 Setup CI/CD GitHub Actions Cho Lab

Repo da co workflow:

```text
.github/workflows/devsecops-platform.yml
```

Workflow nay lam cac viec chinh:

- Test Go backend.
- Build/lint frontend.
- Test/lint AI service.
- Validate Helm chart production profile.
- Validate Helm chart lab 4GB profile.
- Secret scan bang Gitleaks.
- CodeQL scan.
- Trivy filesystem scan.
- Build va push 3 images len GHCR:
  - `ghcr.io/tuananh165-art/nexuschat/api:<tag>`
  - `ghcr.io/tuananh165-art/nexuschat/web:<tag>`
  - `ghcr.io/tuananh165-art/nexuschat/ai-service:<tag>`
- Scan images.
- Tao SBOM.
- Sign images bang Cosign keyless.

### 7.2 Bat Quyen GHCR Cho GitHub Actions

Tren GitHub repository:

1. Vao `Settings`.
2. Vao `Actions` -> `General`.
3. O `Workflow permissions`, chon `Read and write permissions`.
4. Bat `Allow GitHub Actions to create and approve pull requests` neu quy trinh cua ban can bot tao PR.
5. Save.

Workflow da khai bao:

```yaml
permissions:
  contents: read
  packages: write
  security-events: write
  id-token: write
  actions: read
```

Voi GHCR public package, Kubernetes co the pull image ma khong can image pull secret. Voi GHCR private package, phai tao `ghcr-pull-secret` trong namespace `nexuschat-lab`.

### 7.3 Chay Pipeline Thu Cong

Workflow da ho tro `workflow_dispatch`, nen co the chay thu cong:

1. Vao tab `Actions`.
2. Chon `DevSecOps Platform Pipeline`.
3. Bam `Run workflow`.
4. Chon branch `main`.
5. Bam `Run workflow`.

Hoac chi can push len `main`:

```bash
git add .
git commit -m "Configure NexusChat lab deployment"
git push origin main
```

Sau khi workflow thanh cong, images se co tag la full commit SHA.

Lay commit SHA tren server:

```bash
cd /opt/NexusChat
git pull --ff-only
export IMAGE_TAG=$(git rev-parse HEAD)
```

Neu ban build tu release tag, vi du `v0.1.0`, thi dung:

```bash
export IMAGE_TAG='v0.1.0'
```

### 7.4 Kiem Tra Images Tren GHCR

Kiem tra trong GitHub:

1. Vao repository hoac organization/user packages.
2. Mo Packages.
3. Kiem tra co cac package:
   - `nexuschat/api`
   - `nexuschat/web`
   - `nexuschat/ai-service`

Neu packages dang private, tao GitHub token co quyen `read:packages`, sau do tao pull secret:

```bash
kubectl create secret docker-registry ghcr-pull-secret \
  --namespace nexuschat-lab \
  --docker-server=ghcr.io \
  --docker-username=<github-username> \
  --docker-password=<github-token-read-packages> \
  --docker-email=<email> \
  --dry-run=client -o yaml | kubectl apply -f -
```

Khi deploy Helm, them:

```bash
--set global.imagePullSecrets[0].name=ghcr-pull-secret
```

Vi du:

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-4gb.yaml \
  --set imageDefaults.tag="$IMAGE_TAG" \
  --set global.imagePullSecrets[0].name=ghcr-pull-secret
```

### 7.5 Neu Khong Dung GitHub Actions

Neu images da co tren GHCR, chon tag:

```bash
export IMAGE_TAG='<image-tag>'
```

Neu phai build thu cong:

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

## 8. Tao Secret Cho NexusChat

Neu chua dung OAuth/AI that, van can tao placeholder de pod co env day du.

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

## 9. Deploy NexusChat Lab Bang Helm Truc Tiep

Day la cach nhe nhat cho server 4GB. Chua can ArgoCD.

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-4gb.yaml \
  --set imageDefaults.tag="$IMAGE_TAG"
```

Kiem tra:

```bash
kubectl get pods -n nexuschat-lab -o wide
kubectl get svc -n nexuschat-lab
kubectl get ingress -n nexuschat-lab
```

## 10. Truy Cap Bang IP Server

Lay IP server:

```bash
hostname -I
```

Voi `values-lab-4gb.yaml`, `global.domain` dang de trong nen Ingress se khong gan host. Ban co the goi truc tiep bang IP:

```bash
curl -I http://<server-ip>
curl -I http://<server-ip>/api/ai/health
```

Neu ban van muon dung ten `nexuschat.local`, them vao file hosts tren may client:

```text
<server-ip> nexuschat.local
```

Sau do override domain khi deploy:

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

## 11. Theo Doi Tai Nguyen

```bash
kubectl top nodes
kubectl top pods -A
df -h
free -h
```

Neu `kubectl top` chua hoat dong, cai metrics-server:

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl patch deployment metrics-server -n kube-system --type=json \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
```

## 12. Neu Server Bi Qua Tai

Thu tu uu tien giam tai:

1. Tat AI service neu chua test AI:

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-4gb.yaml \
  --set imageDefaults.tag="$IMAGE_TAG" \
  --set services.ai-service.enabled=false
```

2. Dua Postgres, MinIO ra ngoai server.
3. Dua Kafka ra ngoai server.
4. Dua Cassandra ra ngoai server.
5. Chuyen ve Docker Compose hoac tang server len toi thieu 8GB RAM.

## 13. Khong Nen Cai Tren Server 4GB

Khong apply cac file sau neu server chi co 4GB RAM:

```text
deployments/gitops/applications/platform-apps.yaml
deployments/platform/logging/eck-stack.yaml
deployments/platform/consul/values.yaml
deployments/platform/monitoring/kube-prometheus-stack-values.yaml
```

Neu muon dung ArgoCD cho lab, chi cai ArgoCD single replica va deploy app, khong cai toan bo platform apps.

## 14. Cleanup Khi Can

Xoa app:

```bash
helm uninstall nexuschat -n nexuschat-lab
```

Xoa dependencies:

```bash
helm uninstall redis -n redis
helm uninstall kafka -n kafka
helm uninstall cassandra -n cassandra
helm uninstall minio -n minio
helm uninstall ai-postgres -n postgres
```

Xoa K3s:

```bash
sudo /usr/local/bin/k3s-uninstall.sh
```

## Checklist Lab

- K3s Ready.
- Helm Ready.
- Nginx Ingress Ready.
- Redis, Kafka, Cassandra, MinIO, Postgres Ready.
- Images da co tren registry.
- Secret `nexuschat-runtime` da co trong `nexuschat-lab`.
- Helm deploy NexusChat bang `values-lab-4gb.yaml`.
- `curl http://<server-ip>` co response.
