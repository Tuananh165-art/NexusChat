# NexusChat DevSecOps Platform Plan

This document describes the current DevSecOps baseline in source: GitHub Actions build, test, scan, sign Docker Hub images, and deploy the K3s lab directly with Helm. ArgoCD, Consul, ELK, and kube-prometheus-stack still have reference manifests in the repository, but they are not on the current critical lab deployment path.

## Target architecture

- Runtime control plane: Kubernetes/K3s.
- Application packaging: Helm chart `deployments/helm/nexuschat`.
- CI/CD orchestration: `.github/workflows/devsecops-platform.yml`.
- Registry: Docker Hub namespace `docker.io/tuananh165`.
- Lab ingress: ingress-nginx, namespace `nexuschat-lab`, server `192.168.109.131`.
- Security gates: Gitleaks, dependency review, CodeQL, Trivy FS/image, SBOM, Cosign keyless signing.
- Optional platform add-ons: Prometheus/Grafana, Jaeger, ELK/ECK, Kyverno, ArgoCD, Consul.

## Environment model

| Environment | Source | Namespace | Delivery mode | Notes |
| --- | --- | --- | --- | --- |
| `dev` | feature branches / PRs | none or preview | CI validation only | No automatic deploy in current workflow |
| `lab` | push to `main` | `nexuschat-lab` | Direct Helm from self-hosted GitHub Actions runner | Current active CD path |
| `staging` | optional ArgoCD app | `nexuschat-staging` | Optional GitOps manifest | Present in `deployments/gitops/applications/nexuschat-staging.yaml`, not required for lab CD |
| `production` | signed `v*` tag or approved SHA | `nexuschat-prod` or chosen namespace | Manual Helm promotion or optional ArgoCD app | Use immutable image tags only |

## Helm application scope

The chart deploys:

- Deployments/Services for `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, `ai-service`.
- Nginx Ingress resources for services with paths.
- Optional Traefik Middleware resource only when `services.<name>.ingress.traefikForwardAuth.enabled=true`.
- ServiceAccounts, security contexts, resource requests/limits.
- HPA, PDB, ServiceMonitor, NetworkPolicy when enabled.

The chart intentionally does not install Kafka, Redis, Cassandra, MinIO/S3, PostgreSQL, ingress-nginx, cert-manager, Prometheus/Grafana, ELK, Consul, or ArgoCD. Those are platform dependencies and must be installed separately.

## Current pipeline

### Triggers

- `pull_request` to any branch: validation jobs.
- `push` to `main` or `kafka`: validation + image build/scan/sign. `main` additionally deploys lab.
- `push` tag `v*`: validation + image build/scan/sign for release tag.
- `workflow_dispatch`: manual run.

### Jobs

| Job | Purpose |
| --- | --- |
| `test` | Go `make test`, frontend `npm ci`/lint/build, AI service install/ruff/pytest |
| `helm` | Helm lint, default render, lab 4GB render, upload rendered manifests |
| `secret-scan` | Gitleaks scan |
| `dependency-review` | PR-only dependency review |
| `codeql` | Go, JavaScript/TypeScript, Python CodeQL |
| `trivy-fs` | Repository filesystem vulnerability scan |
| `build-images` | Build/push/scan/SBOM/sign `nexuschat-api`, `nexuschat-web`, `nexuschat-ai-service` |
| `build-proxy-variants` | Build/push lab-specific proxy variant tags for API/web |
| `deploy-lab-k8s` | Self-hosted runner deploy to `nexuschat-lab` with Helm and verify rollouts |

## Image and tag model

Primary images:

- `docker.io/tuananh165/nexuschat-api:<tag>`
- `docker.io/tuananh165/nexuschat-web:<tag>`
- `docker.io/tuananh165/nexuschat-ai-service:<tag>`

Additional lab/proxy variants:

- `docker.io/tuananh165/nexuschat-api:proxy-upload`
- `docker.io/tuananh165/nexuschat-api:proxy-upload-<tag>`
- `docker.io/tuananh165/nexuschat-web:proxy-upload`
- `docker.io/tuananh165/nexuschat-web:proxy-upload-<tag>`
- `docker.io/tuananh165/nexuschat-web:proxy-upload-v2`
- `docker.io/tuananh165/nexuschat-web:proxy-upload-v2-<tag>`

Tag is `GITHUB_SHA` for branch pushes and `GITHUB_REF_NAME` for tags. Docker metadata action also creates `sha-*` metadata tags for primary images.

## Lab deployment detail

The `deploy-lab-k8s` job runs only for `push` to `main` on a self-hosted runner with labels:

```text
[self-hosted, linux, x64, k3s-lab]
```

It:

1. Checks out source.
2. Sets `TAG=${GITHUB_SHA}`.
3. Installs/sets up kubectl and Helm.
4. Writes `~/.kube/config` from `KUBE_CONFIG_B64` if the secret is provided; otherwise uses the runner's existing kubeconfig.
5. Prints current cluster context/server and nodes.
6. Ensures namespace `nexuschat-lab` exists.
7. Runs `helm upgrade --install nexuschat ... --wait --timeout 10m` with base values plus `values-lab-k3s.yaml`.
8. Overrides `imageDefaults.tag` to the SHA.
9. Overrides `web` and `uploader` to SHA-specific proxy variant images.
10. Waits for deployments `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, `ai-service`.
11. Prints actual images from live Deployments.

## Required secrets

| Secret | Used by | Required |
| --- | --- | --- |
| `DOCKER_USERNAME` | Docker Hub login | yes |
| `DOCKER_PASSWORD` | Docker Hub login | yes |
| `KUBE_CONFIG_B64` | Lab deploy kubeconfig | required unless self-hosted runner already has kubeconfig |

Create kubeconfig secret from a machine that can manage the target cluster:

```bash
base64 -w0 ~/.kube/config
```

If using GitHub-hosted runners, the Kubernetes API must be reachable from GitHub. Current workflow uses self-hosted lab runner, which is the preferred path for private/LAN K3s.

## Runtime secret model

The app chart defaults to `global.envFromSecrets: [nexuschat-runtime]`. Create this secret in each application namespace outside Git.

Required or commonly required keys:

- `CHAT_JWT_SECRET`
- `REDIS_PASSWORD`
- `CASSANDRA_USER`
- `CASSANDRA_PASSWORD`
- `UPLOADER_S3_ACCESSKEY`
- `UPLOADER_S3_SECRETKEY`
- `USER_OAUTH_GOOGLE_CLIENTID`
- `USER_OAUTH_GOOGLE_CLIENTSECRET`
- `DATABASE_URL`
- `AI_ENDPOINT`
- `AI_API_KEY`
- `AI_MODEL`
- `AI_POSTGRES_PASSWORD`

## Security baseline

Repository/build controls:

- Branch protection on `main`.
- Required PR reviews for application/deployment changes.
- Gitleaks secret scan.
- Dependency review on PRs.
- CodeQL for Go, TypeScript/JavaScript, Python.
- Trivy filesystem and image scans.
- SPDX JSON SBOM artifacts.
- Cosign keyless image signing with GitHub OIDC.

Runtime controls in chart:

- Non-root pod/container security context.
- No privilege escalation.
- Drop all capabilities.
- Read-only root filesystem.
- Requests/limits per container.
- Optional NetworkPolicy and Kyverno policies.
- TLS at ingress for non-lab domains.

## Observability baseline

- Go services expose metrics using configured `OBSERVABILITY_PROMETHEUS_PORT` (default `8080`).
- `ai-service` exposes `/metrics` on port `8090`.
- Chart can emit ServiceMonitor resources when Prometheus Operator is installed.
- Local Compose includes Prometheus and Jaeger.
- Platform manifests include standalone/kube-prometheus-stack references, Grafana ingress, Prometheus ingress, Jaeger ingress, and optional ELK/ECK manifests.

## Promotion and rollback

Lab auto-deploys every successful push to `main`.

Production should be explicit:

```bash
export IMAGE_TAG='<approved-sha-or-v-tag>'
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-prod \
  --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --set-string imageDefaults.tag="$IMAGE_TAG" \
  --set-string global.environment=production \
  --set-string global.domain=nexuschat.click \
  --wait --timeout 10m
```

Rollback:

```bash
helm history nexuschat -n nexuschat-lab
helm rollback nexuschat <REVISION> -n nexuschat-lab
```

or redeploy the previous immutable image tag with `helm upgrade --install`.
