# Kubernetes manifests

Apply these manifests to deploy ChromaFlow plus Prometheus, Grafana, Loki, and Promtail into the `chromaflow` namespace:

```sh
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/
```

Before applying in a real cluster, replace `ghcr.io/OWNER/REPO:latest` in `chromaflow.yaml` with the published image for your fork or repository.

The manifests are intentionally plain YAML so they work in many Kubernetes-compatible environments. For production, replace `emptyDir` storage with persistent volumes, set real hostnames/TLS on ingress resources, and move Grafana credentials to your secret manager.

Future Redis, RabbitMQ/Kafka, MinIO, and token-auth configuration should be added as separate manifests or a Helm chart so the core single-node deployment remains easy to run.
