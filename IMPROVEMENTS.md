# Improvements

ChromaFlow now has baseline CI/CD, release artifacts, health endpoints, Prometheus-format metrics, structured JSON logs, OpenAPI documentation, Docker Compose observability, Kubernetes manifests, Redis-backed queue/result metadata, MinIO-compatible PDF object storage, idempotency keys, cancellation endpoints, reusable Chromium, webhook callbacks, alert rules, graceful worker shutdown, machine-readable errors, configurable log levels, request IDs, render retries, local Chromium docs, and packaging metadata. The items below are the next production-readiness milestones.

## Reliability

- Add more tests for worker success/failure paths, websocket snapshots, PDF generation abstractions, Redis/S3 integration, and configuration parsing.
- Replace the in-memory result map with TTL eviction and limits on total stored PDF bytes.

## Security

- Add full SSRF protections: block loopback, link-local, private networks, metadata IPs, internal hostnames, DNS rebinding, and redirects to forbidden addresses.
- Add token-based authentication and authorization before exposing the service outside a trusted network; update OpenAPI security schemes, Docker Compose, and Kubernetes secrets at the same time.
- Add explicit rate limiting per client/API key.
- Limit maximum PDF output size and total in-memory result storage.
- Review Chromium sandbox settings for each deployment target. The current launcher uses `NoSandbox(true)` for compatibility with many container environments.

## Scalability

- Evaluate RabbitMQ or Kafka for asynchronous job lifecycle events and integration streams after the Redis/object-storage/webhook design is stable.
- Expand the reusable browser implementation into a configurable browser pool with health checks and per-worker isolation.
- Expand metrics with worker utilization, timeout/error classifications, Chromium launch/connect timings, and autoscaling-friendly saturation signals.

## Product/API

- Add PDF options such as page size, margins, landscape mode, print background, and CSS page size preference.
- Add additional Chrome-powered job types behind a versioned API, such as screenshots, page metadata extraction, and accessibility snapshots.
- Add dashboard filtering/search and per-job detail views.
- Add an endpoint to delete a result before TTL expiry.
- Include `started_at`, `finished_at`, and duration fields in job status responses.

## Observability

- Add a richer Grafana dashboard for HTTP errors, queue saturation, render percentiles, PDF byte growth, and log drilldowns.
- Consider OpenTelemetry traces to extend the current request ID propagation across browsers, Redis, object storage, and webhooks.

## Packaging and operations

- Add signed release artifacts and container provenance attestations.
- Add SBOM generation for release artifacts.
- Publish example systemd and Windows service configurations for direct binary deployments.
- Add a Helm chart or Kustomize overlays for the existing Kubernetes manifests, including production persistent volumes, TLS ingress, resource presets, and optional Redis/RabbitMQ/Kafka/MinIO dependencies.
