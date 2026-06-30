# NexusChat DevSecOps Implementation Runbook

This runbook is the step-by-step implementation guide for the DevSecOps platform assets in this repository.

## 1. Repository Preparation

1. Protect `main` and release tag workflows in GitHub.
2. Require the `DevSecOps Platform Pipeline` checks before merge.
3. Enable GitHub Advanced Security or equivalent controls for CodeQL, secret scanning, and dependency alerts.
4. Configure container registry permissions for GitHub Actions OIDC and GHCR package writes.
5. Create repository or organization secrets only for external systems that cannot use OIDC.

Required GitHub settings:

- `id-token: write` allowed for workflows that sign images.
- GHCR package write permission for `GITHUB_TOKEN`.
- Branch protection requiring reviews from application and DevSecOps owners.

## 2. Cluster Bootstrap

Create namespaces:

```bash
kubectl create namespace argocd
kubectl create namespace ingress-nginx
kubectl create namespace consul
kubectl create namespace monitoring
kubectl create namespace logging
kubectl create namespace security
kubectl create namespace nexuschat-staging
kubectl create namespace nexuschat-prod
```

Install ArgoCD:

```bash
helm repo add argo https://argoproj.github.io/argo-helm
helm upgrade --install argocd argo/argo-cd \
  --namespace argocd \
  --values deployments/platform/argocd/values.yaml
```

Apply the root platform applications:

```bash
kubectl apply -f deployments/gitops/applications/platform-apps.yaml
kubectl apply -f deployments/gitops/applications/nexuschat-staging.yaml
```

Production should be applied only after staging smoke tests pass:

```bash
kubectl apply -f deployments/gitops/applications/nexuschat-production.yaml
```

## 3. Secrets

Production secrets must not be committed to Git.

Create or sync a Kubernetes secret named `nexuschat-runtime` in each application namespace with these keys:

- `CHAT_JWT_SECRET`
- `REDIS_PASSWORD`
- `CASSANDRA_USER`
- `CASSANDRA_PASSWORD`
- `UPLOADER_S3_ACCESSKEY`
- `UPLOADER_S3_SECRETKEY`
- `USER_OAUTH_GOOGLE_CLIENTID`
- `USER_OAUTH_GOOGLE_CLIENTSECRET`
- `AI_POSTGRES_PASSWORD`
- AI provider credentials required by `ai-service`

Recommended production implementation:

1. Store source secrets in a cloud secret manager.
2. Install External Secrets Operator.
3. Bind `ExternalSecret` resources to create `nexuschat-runtime`.
4. Rotate credentials through the secret manager, not through Git.

## 4. Image and Release Promotion

1. Merge application changes to `main`.
2. Confirm workflow jobs pass: tests, Helm validation, scans, SBOM, image build, image signing.
3. Confirm staging ArgoCD sync is healthy.
4. Run smoke tests against `https://staging.nexuschat.example.com`.
5. Create a signed release tag:

```bash
git tag -s v0.1.0 -m "NexusChat v0.1.0"
git push origin v0.1.0
```

6. Update `deployments/gitops/applications/nexuschat-production.yaml` to the release tag and chart image tag.
7. Merge the promotion change and sync the production app.

## 5. Nginx Ingress

1. Install `ingress-nginx` through ArgoCD.
2. Point DNS records to the ingress controller load balancer:
   - `staging.nexuschat.example.com`
   - `nexuschat.example.com`
   - `argocd.nexuschat.example.com`
3. Configure cert-manager or a cloud certificate controller to create TLS secrets.
4. Confirm ingress routes:

```bash
kubectl get ingress -n nexuschat-staging
kubectl describe ingress nexuschat -n nexuschat-staging
```

## 6. Observability

1. Install kube-prometheus-stack through ArgoCD.
2. Apply NexusChat alert rules from `deployments/platform/monitoring/nexuschat-rules.yaml`.
3. Confirm ServiceMonitor discovery:

```bash
kubectl get servicemonitor -n nexuschat-staging
kubectl port-forward -n monitoring svc/kube-prometheus-stack-grafana 3000:80
```

4. In Grafana, verify:
   - Kubernetes pod health.
   - Nginx ingress latency and 5xx rate.
   - NexusChat service request rates and error rates.
   - Consul sidecar and service mesh metrics.

## 7. ELK Logging

1. Install ECK or the selected Elastic distribution in `logging`.
2. Apply `deployments/platform/logging/eck-stack.yaml`.
3. Confirm Elasticsearch, Kibana, and Filebeat are healthy:

```bash
kubectl get elasticsearch,kibana,beat -n logging
kubectl get pods -n logging
```

4. Create Kibana data views for `filebeat-*`.
5. Add saved searches for NexusChat namespaces and Nginx ingress logs.

## 8. Consul Service Mesh

1. Install Consul using `deployments/platform/consul/values.yaml`.
2. Confirm injector and controller pods are healthy.
3. Deploy NexusChat chart with Consul annotations enabled.
4. Confirm sidecars are injected:

```bash
kubectl get pods -n nexuschat-staging -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.spec.containers[*].name}{"\n"}{end}'
```

5. Add service intentions before enforcing deny-by-default mesh policy.

## 9. Security Admission

1. Install Kyverno in `security`.
2. Apply `deployments/platform/security/kyverno-policies.yaml`.
3. Run in `Audit` mode first.
4. Fix violations.
5. Move production policies to `Enforce` after two clean releases.

## 10. Rollback

Application rollback:

```bash
argocd app history nexuschat-production
argocd app rollback nexuschat-production <revision-id>
```

Image rollback:

1. Set `imageDefaults.tag` to the last known good immutable tag.
2. Commit the GitOps change.
3. Let ArgoCD sync the production app.

Emergency manual rollback is allowed only during an active incident and must be backfilled into Git before incident closure.
