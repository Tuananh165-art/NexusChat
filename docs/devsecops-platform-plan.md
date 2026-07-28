# Current DevSecOps Platform

## CI

Workflow `.github/workflows/devsecops-platform.yml` runs:

- Go unit tests and race tests.
- Frontend `npm ci`, lint, and build.
- AI service Ruff and pytest.
- Helm lint/template.
- Gitleaks secret scanning.
- Dependency Review on Pull Requests.
- CodeQL for Go, JavaScript/TypeScript, and Python.
- Trivy filesystem scanning.

The workflow triggers on `workflow_dispatch`, `pull_request` to any branch, pushes to `main` or `kafka`, and `v*` tags.

## Container security gates

All application images and proxy variants:

1. Build locally on the runner.
2. Run a Trivy scan with `CRITICAL,HIGH` and `exit-code: 1`.
3. Generate an SPDX SBOM.
4. Upload SARIF/SBOM artifacts.
5. Push only after the gate passes.
6. Sign the immutable image with Cosign.

Proxy variants:

- `nexuschat-api:proxy-upload-<sha>`.
- `nexuschat-web:proxy-upload-<sha>`.
- `nexuschat-web:proxy-upload-v2-<sha>`.

## CD

CD is currently direct Helm:

- Deploys to the lab after the required image jobs succeed for a push to the `main` branch or a manual `workflow_dispatch`.
- The Kubernetes deploy job requires the `main` push/manual-dispatch condition, `build-images`, and `build-proxy-variants` to pass.
- The deploy job requires `build-images` and `build-proxy-variants` to pass.
- Runner: `[self-hosted, linux, x64, k3s-lab]`.
- App namespace: `nexuschat-lab`.
- Deploys platform observability with `deployments/platform/observability`.
- Deploys Kafka UI/RedisInsight with `deployments/platform/dashboards`.
- Deploys the application with Helm using `values.yaml` followed by `values-lab-k3s.yaml`.
- Waits for ten Deployments: `web`, `chat`, `match`, `user`, `uploader`, `forwarder`, `ai-service`, `safety`, `discovery`, `workspace`.
- Performs `/ready` smoke checks for Safety, Discovery, and Workspace.

The workflow does not install Kafka, Redis, Cassandra, MinIO, PostgreSQL, Traefik, kube-prometheus-stack, or Kyverno. Platform dependencies must be prepared in the cluster first.

A `git pull` alone does not deploy. A `git commit` followed by `git push` to `main` can trigger CD.

## Observability

- Prometheus/Grafana: kube-prometheus-stack, with NodePorts `30900` and `30300`, respectively.
- Jaeger UI: `jaeger-ui` Service, NodePort `30686`.
- OpenTelemetry Collector: internal Service `otel-collector.monitoring.svc.cluster.local:4317/4318`.
- Kafka UI: NodePort `30080`.
- RedisInsight: NodePort `30540`.

NodePorts are for a lab/protected network. Production dashboards require authentication, firewall/VPN controls, and TLS through Traefik.

## Kubernetes security

Kyverno policies are located at `deployments/platform/security/kyverno-policies.yaml`. Install Kyverno before applying the policy. Check `validationFailureAction` and the namespace match before changing from Audit to Enforce.

The app Helm chart enables a non-root security context, drops capabilities, uses a read-only root filesystem, and sets resource limits in the template. The app NetworkPolicy/ServiceMonitor are disabled in the lab profile.

## Remaining limitations

- CD still depends on a self-hosted runner and `KUBE_CONFIG_B64`.
- Stateful platform services are not provisioned automatically by the workflow.
- NodePorts are not suitable for public production.
- The workflow has no production promotion/approval yet.
- Helm lint/template requires a machine with Helm; cluster rollout requires a working kubeconfig.

