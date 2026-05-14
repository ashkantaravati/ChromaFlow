# Kubernetes manifests

Apply these manifests to deploy ChromaFlow, example Redis/MinIO dependencies, Prometheus, Grafana, Loki, and Promtail into the `chromaflow` namespace:

```sh
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/
```

Before applying in a real cluster, replace `ghcr.io/OWNER/REPO:latest` in `chromaflow.yaml` with the published image for your fork or repository.

The included Redis and MinIO manifests are development-friendly examples for the shared queue/result and object-storage backends. For production, use managed Redis/S3-compatible services or add persistent volumes, credentials from Secrets, backup policies, TLS, real hostnames, and resource limits that match your workload.

The manifests are intentionally plain YAML so they work in many Kubernetes-compatible environments. A future Helm chart or Kustomize overlay can layer production storage classes, secret management, ingress, and autoscaling over these defaults.
