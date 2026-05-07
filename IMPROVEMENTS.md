# Improvements

ChromaFlow now has baseline CI/CD, release artifacts, health endpoints, and packaging metadata. The items below are the next production-readiness milestones.

## Reliability

- Add tests for worker success/failure paths, websocket snapshots, PDF generation abstractions, and configuration parsing.
- Replace the in-memory result map with TTL eviction and limits on total stored PDF bytes.
- Add a `sync.WaitGroup` to the worker pool so graceful shutdown waits for in-flight jobs or marks them failed cleanly.
- Return richer machine-readable error payloads instead of plain text `http.Error` responses.
- Add structured logging with job IDs, duration, URL host, output size, worker ID, and failure reason.
- Add retry policies for transient Chromium/page failures.

## Security

- Add full SSRF protections: block loopback, link-local, private networks, metadata IPs, internal hostnames, DNS rebinding, and redirects to forbidden addresses.
- Consider request authentication and authorization before exposing the service outside a trusted network.
- Add explicit rate limiting per client/API key.
- Limit maximum PDF output size and total in-memory result storage.
- Review Chromium sandbox settings for each deployment target. The current launcher uses `NoSandbox(true)` for compatibility with many container environments.

## Scalability

- Add a Redis-backed queue/result backend so jobs survive restarts and multiple ChromaFlow instances can process one shared queue.
- Store generated PDFs in object storage or durable disk instead of process memory.
- Reuse a browser instance or browser pool to avoid launching Chromium for every job.
- Add queue depth, worker utilization, render duration, and error metrics for autoscaling and operational visibility.
- Add job cancellation and idempotency keys.

## Product/API

- Add PDF options such as page size, margins, landscape mode, print background, and CSS page size preference.
- Add additional Chrome-powered job types behind a versioned API, such as screenshots, page metadata extraction, and accessibility snapshots.
- Add webhook callbacks for completed or failed jobs.
- Add dashboard filtering/search and per-job detail views.
- Add an endpoint to delete a result before TTL expiry.
- Include `started_at`, `finished_at`, and duration fields in job status responses.

## Packaging and operations

- Add signed release artifacts and container provenance attestations.
- Add SBOM generation for release artifacts.
- Publish example systemd and Windows service configurations for direct binary deployments.
- Add Kubernetes manifests or a Helm chart once the Redis backend is available.
- Document local Chromium setup for Linux/macOS/Windows more thoroughly.
