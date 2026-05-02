# Improvements

## Reliability

- Add tests for API status transitions, queue overflow behavior, worker success/failure paths, and configuration parsing.
- Replace the in-memory result map with TTL eviction and limits on total stored PDF bytes.
- Add a `sync.WaitGroup` to the worker pool so graceful shutdown waits for in-flight jobs or marks them failed cleanly.
- Return `202 Accepted` for submitted jobs and clearer status codes for failed/unknown jobs.
- Add structured logging with job IDs, duration, URL host, output size, and failure reason.

## Security

- Validate submitted URLs before enqueueing: allow only `http` and `https` unless explicitly configured otherwise.
- Add SSRF protections: block loopback, link-local, private networks, metadata IPs, and redirects to forbidden addresses.
- Consider request authentication or rate limiting before exposing the service outside a trusted network.
- Limit input URL length and JSON request body size.
- Run Chromium with the least privileges possible and review whether `NoSandbox(true)` is only used inside trusted containers.

## Scalability

- Add a durable queue such as Redis, Postgres, NATS, or another backend so jobs survive restarts.
- Store generated PDFs in object storage or on disk instead of process memory.
- Reuse a browser instance or a browser pool to avoid launching Chromium for every job.
- Add queue depth and worker metrics for autoscaling and operational visibility.
- Add job cancellation and retry policies.

## Product/API

- Add PDF options such as page size, margins, landscape mode, print background, and CSS page size preference.
- Add webhook callbacks for completed or failed jobs.
- Add a health endpoint and readiness endpoint.
- Add an endpoint to delete a result before TTL expiry.
- Include `created_at`, `started_at`, `finished_at`, and duration fields in job status responses.

## Packaging

- Add a `.dockerignore` to keep build contexts small.
- Pin the Alpine base image version instead of using `alpine:latest`.
- Document local Chromium setup for Linux/macOS more thoroughly.
- Add CI that runs `go test ./...` and builds the Docker image.
