# NexusChat CI/CD Guide: GitHub Actions + Docker Hub + Direct K8s Deploy

This document describes the current source-accurate flow. The lab CD path does not depend on ArgoCD; ArgoCD remains only as optional/reference manifests under `deployments/gitops/applications`.

## 1. Current standard flow

When code is pushed to `main`:

1. GitHub Actions runs validation: Go tests, frontend lint/build, AI service lint/tests, Helm lint/template, and security scans.
2. Primary images are built and pushed to Docker Hub.
3. Additional lab/proxy upload image variants are built.
4. Images are scanned, SBOMs are created, and images are signed with Cosign keyless signing.
5. The `deploy-lab-k8s` job runs on a self-hosted runner with the `k3s-lab` label.
6. The job uses kubeconfig from `KUBE_CONFIG_B64` when provided, or the runner's existing kubeconfig otherwise.
7. It runs `helm upgrade --install` into namespace `nexuschat-lab`.
8. It waits for these Deployments: `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, `ai-service`.
9. It prints the actual images running in the cluster.

## 2. Main files

| File | Role |
| --- | --- |
| `.github/workflows/devsecops-platform.yml` | Main CI/CD pipeline |
| `deployments/helm/nexuschat` | Application Helm chart |
| `deployments/helm/nexuschat/values.yaml` | Default values |
| `deployments/helm/nexuschat/values-lab-k3s.yaml` | Overrides for the small K3s lab |
| `docs/deploy-k8s-guide.md` | Detailed lab deployment runbook |

## 3. Images built and pushed

Primary images:

- `docker.io/tuananh165/nexuschat-api:<TAG>`
- `docker.io/tuananh165/nexuschat-web:<TAG>`
- `docker.io/tuananh165/nexuschat-ai-service:<TAG>`

Lab/proxy upload variants:

- `docker.io/tuananh165/nexuschat-api:proxy-upload`
- `docker.io/tuananh165/nexuschat-api:proxy-upload-<TAG>`
- `docker.io/tuananh165/nexuschat-web:proxy-upload`
- `docker.io/tuananh165/nexuschat-web:proxy-upload-<TAG>`
- `docker.io/tuananh165/nexuschat-web:proxy-upload-v2`
- `docker.io/tuananh165/nexuschat-web:proxy-upload-v2-<TAG>`

`TAG` is:

- `GITHUB_SHA` for branch pushes.
- `GITHUB_REF_NAME` for `v*` tag pushes.

## 4. Required GitHub Actions secrets

Create these in the GitHub repository under `Settings` -> `Secrets and variables` -> `Actions`.

| Secret | Purpose |
| --- | --- |
| `DOCKER_USERNAME` | Docker Hub login |
| `DOCKER_PASSWORD` | Docker Hub access token/password |
| `KUBE_CONFIG_B64` | Base64 kubeconfig for lab deploy; can be empty only if the self-hosted runner already has kubeconfig |

Create `KUBE_CONFIG_B64`:

```bash
base64 -w0 ~/.kube/config
```

The current lab should use a self-hosted runner on the same network as K3s. The workflow requires these labels:

```text
self-hosted, linux, x64, k3s-lab
```

## 5. Deploy command run by the workflow

In the `deploy-lab-k8s` job, the current Helm command is equivalent to:

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-k3s.yaml \
  --set-string imageDefaults.tag="${GITHUB_SHA}" \
  --set-string services.web.image.fullname="docker.io/tuananh165/nexuschat-web:proxy-upload-v2-${GITHUB_SHA}" \
  --set-string services.uploader.image.fullname="docker.io/tuananh165/nexuschat-api:proxy-upload-${GITHUB_SHA}" \
  --timeout 10m \
  --wait
```

Actual lab image mapping:

| Service | Lab image |
| --- | --- |
| `web` | `docker.io/tuananh165/nexuschat-web:proxy-upload-v2-<SHA>` |
| `uploader` | `docker.io/tuananh165/nexuschat-api:proxy-upload-<SHA>` |
| `chat` | `docker.io/tuananh165/nexuschat-api:<SHA>` |
| `match` | `docker.io/tuananh165/nexuschat-api:<SHA>` |
| `user` | `docker.io/tuananh165/nexuschat-api:<SHA>` |
| `forwarder` | `docker.io/tuananh165/nexuschat-api:<SHA>` |
| `ai-service` | `docker.io/tuananh165/nexuschat-ai-service:<SHA>` |

## 6. Checks after push

Watch workflow runs:

```bash
gh run list --workflow "DevSecOps Platform Pipeline" --limit 5
gh run watch
```

Check the cluster:

```bash
kubectl -n nexuschat-lab get pods -o wide
kubectl -n nexuschat-lab get deploy
kubectl -n nexuschat-lab get ingress
kubectl -n nexuschat-lab get deploy -o jsonpath='{range .items[*]}{.metadata.name}{" => "}{range .spec.template.spec.containers[*]}{.image}{" "}{end}{"\n"}{end}'
```

Rollout status for the current Deployment names:

```bash
for deploy in web chat match user uploader forwarder ai-service; do
  kubectl -n nexuschat-lab rollout status deployment/$deploy --timeout=600s
done
```

Smoke test the current lab IP:

```bash
curl -I http://192.168.109.131
curl -i http://192.168.109.131/api/ai/health
```

## 7. Quick rollback

```bash
helm history nexuschat -n nexuschat-lab
helm rollback nexuschat <REVISION> -n nexuschat-lab
```

Or redeploy an old tag:

```bash
export OLD_TAG='<old-git-sha>'
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab \
  --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-k3s.yaml \
  --set-string imageDefaults.tag="$OLD_TAG" \
  --set-string services.web.image.fullname="docker.io/tuananh165/nexuschat-web:proxy-upload-v2-$OLD_TAG" \
  --set-string services.uploader.image.fullname="docker.io/tuananh165/nexuschat-api:proxy-upload-$OLD_TAG" \
  --wait --timeout 10m
```

## 8. Recommended commit/push commands

Because local working trees can include modified/untracked files, avoid `git add .` until all changes are reviewed. For docs-only changes:

```bash
git status --short
git diff -- README.md docs/*.md ai-service/README.md deployments/platform/UI-ACCESS.md
git add README.md docs/*.md ai-service/README.md deployments/platform/UI-ACCESS.md
git commit -m "docs: update architecture and k8s deployment pipeline"
git push origin main
```

Use `git add .` only after confirming there are no secrets, build artifacts, or unrelated untracked files.

## 9. What is no longer on the critical lab CD path

- ArgoCD sync is not required to deploy the lab.
- Committing image tag bumps to GitOps apps is not required for the lab to receive new images.
- Consul CRDs are not required in the lab profile because `values-lab-k3s.yaml` disables `consul.serviceDefaults.enabled`.
- The Traefik ingress controller is not required in the K3s lab; the lab chart uses ingress-nginx.
