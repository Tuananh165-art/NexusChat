# Docker Hub trực tiếp và rollout K3s

CI push các image theo Git SHA:

- `tuananh165/nexuschat-api:<sha>`
- `tuananh165/nexuschat-web:<sha>`
- `tuananh165/nexuschat-ai-service:<sha>`
- `tuananh165/nexuschat-safety:<sha>`
- `tuananh165/nexuschat-discovery:<sha>`
- `tuananh165/nexuschat-workspace:<sha>`

Workflow không dùng `latest`. Tag release `v*` được giữ nguyên theo tên tag; branch dùng immutable commit SHA.

Helm values nguồn:

- `deployments/helm/nexuschat/values.yaml`
- `deployments/helm/nexuschat/values-lab-k3s.yaml`

Profile lab deploy HTTP-only với ingress-nginx, `tlsSecretName: ""` và `ssl-redirect: false`. Không cài cert-manager, không cài Coturn và không tạo TLS Secret.

```bash
helm upgrade --install nexuschat deployments/helm/nexuschat \
  --namespace nexuschat-lab --create-namespace \
  --values deployments/helm/nexuschat/values.yaml \
  --values deployments/helm/nexuschat/values-lab-k3s.yaml \
  --set-string imageDefaults.tag="$GITHUB_SHA" \
  --atomic --wait --timeout 10m
```

Rollout phải chờ:

`web chat match user uploader forwarder ai-service safety discovery workspace`

Smoke endpoint:

- `http://safety:5005/ready`
- `http://discovery:5006/ready`
- `http://workspace:5007/ready`

Nếu rollout fail, dùng `helm history` và `helm rollback`; không xóa namespace hoặc PVC trong thao tác rollback.
