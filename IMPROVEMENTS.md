# Improvements

ChromaFlow now has baseline CI/CD, release artifacts, health endpoints, Prometheus-format metrics, structured JSON logs, OpenAPI documentation, Docker Compose observability, Kubernetes manifests, and packaging metadata. The items below are the next production-readiness milestones.

## Reliability

- Add tests for worker success/failure paths, websocket snapshots, PDF generation abstractions, and configuration parsing.
- Replace the in-memory result map with TTL eviction and limits on total stored PDF bytes.
- Add a `sync.WaitGroup` to the worker pool so graceful shutdown waits for in-flight jobs or marks them failed cleanly.
- Return richer machine-readable error payloads instead of plain text `http.Error` responses.
- Add configurable log levels and request IDs/tracing context to the existing structured JSON logs.
- Add retry policies for transient Chromium/page failures.

## Security

- Add full SSRF protections: block loopback, link-local, private networks, metadata IPs, internal hostnames, DNS rebinding, and redirects to forbidden addresses.
- Add token-based authentication and authorization before exposing the service outside a trusted network; update OpenAPI security schemes, Docker Compose, and Kubernetes secrets at the same time.
- Add explicit rate limiting per client/API key.
- Limit maximum PDF output size and total in-memory result storage.
- Review Chromium sandbox settings for each deployment target. The current launcher uses `NoSandbox(true)` for compatibility with many container environments.

## Scalability

- Add a Redis-backed queue/result backend so jobs survive restarts and multiple ChromaFlow instances can process one shared queue.
- Store generated PDFs in object storage such as MinIO or durable disk instead of process memory.
- Evaluate RabbitMQ or Kafka for asynchronous job lifecycle events, webhooks, and integration streams after the Redis/object-storage design is stable.
- Reuse a browser instance or browser pool to avoid launching Chromium for every job.
- Expand metrics with worker utilization, timeout/error classifications, Chromium launch/connect timings, and autoscaling-friendly saturation signals.
- Add job cancellation and idempotency keys.

## Product/API

- Add PDF options such as page size, margins, landscape mode, print background, and CSS page size preference.
- Add additional Chrome-powered job types behind a versioned API, such as screenshots, page metadata extraction, and accessibility snapshots.
- Add webhook callbacks for completed or failed jobs.
- Add dashboard filtering/search and per-job detail views.
- Add an endpoint to delete a result before TTL expiry.
- Include `started_at`, `finished_at`, and duration fields in job status responses.

## Observability

- Add alert rules for high queue depth, high render error rate, p95 render latency, worker starvation, and storage byte growth.
- Add a richer Grafana dashboard for HTTP errors, queue saturation, render percentiles, PDF byte growth, and log drilldowns.
- Consider OpenTelemetry traces once request IDs and cross-component context propagation exist.

## Packaging and operations

- Add signed release artifacts and container provenance attestations.
- Add SBOM generation for release artifacts.
- Publish example systemd and Windows service configurations for direct binary deployments.
- Add a Helm chart or Kustomize overlays for the existing Kubernetes manifests, including production persistent volumes, TLS ingress, resource presets, and optional Redis/RabbitMQ/Kafka/MinIO dependencies.
- Document local Chromium setup for Linux/macOS/Windows more thoroughly.
