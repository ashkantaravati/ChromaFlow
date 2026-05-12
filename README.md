# ChromaFlow

ChromaFlow is a Go HTTP service for turning web pages into PDF files with headless Chromium. Clients submit URL jobs through the API or the built-in dashboard, workers render each page with [go-rod](https://github.com/go-rod/rod), and the completed PDF is served from the job endpoint.

The current service is intentionally small and self-contained: queue state and generated PDFs live in memory. It is suitable for single-node deployments today and is structured so a Redis-backed shared queue/result backend can be added for horizontal scaling later.

## Features

- JSON API for URL-to-PDF job submission.
- Browser dashboard at `/` for submitting jobs and watching state changes.
- WebSocket job snapshot stream at `/ws/jobs`.
- In-memory queue and result store with explicit full-queue responses.
- Configurable worker count, queue size, and render timeout.
- URL validation that accepts only absolute `http` and `https` URLs.
- Headless Chromium rendering through go-rod.
- Health, readiness, version, OpenAPI, and Prometheus metrics endpoints for operations.
- Structured JSON logs written to stdout with job, worker, host, duration, and PDF-size fields.
- Docker Compose observability stack with Prometheus, Grafana, Loki, and Promtail.
- Kubernetes manifests for ChromaFlow and the monitoring stack.
- Container image with Chromium installed.
- Linux and Windows binary release workflow.

## Architecture

```text
client/dashboard -> HTTP API -> in-memory queue -> worker pool -> Chromium -> in-memory result store
       ^                              |                                |
       |                              v                                v
       +---------------------- WebSocket snapshots <------------- status/PDF endpoint
```

Important directories:

| Path | Purpose |
| --- | --- |
| `cmd/server/main.go` | Service wiring, routes, workers, graceful shutdown, version injection. |
| `internal/api/handler.go` | Dashboard, PDF job API, health/readiness handlers, websocket route. |
| `internal/config/config.go` | Environment variable configuration. |
| `internal/observability/` | Prometheus-format metrics and structured JSON logging setup. |
| `internal/pdf/generator.go` | Chromium launch/connect and PDF rendering. |
| `internal/queue/` | In-memory queue and job types. |
| `internal/realtime/` | Minimal stdlib websocket hub for job snapshots. |
| `internal/storage/` | In-memory result storage and job snapshot listing. |
| `internal/worker/` | Worker pool and job processing. |
| `observability/` | Prometheus, Loki, Promtail, and Grafana provisioning used by Docker Compose. |
| `k8s/` | Plain Kubernetes YAML for ChromaFlow, services, ingress, Prometheus, Grafana, Loki, and Promtail. |

## API

### Submit a PDF job

```sh
curl -i -X POST http://localhost:8080/pdf \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com"}'
```

Successful submissions return `202 Accepted`:

```json
{
  "job_id": "...",
  "status_url": "/pdf/..."
}
```

Validation and capacity errors:

- `400 Bad Request` for invalid JSON, empty URLs, relative URLs, or schemes other than `http`/`https`.
- `503 Service Unavailable` when the in-memory queue is full.

### Fetch job status or PDF

```sh
curl -OJ http://localhost:8080/pdf/<job_id>
```

If the job is pending, processing, or failed, the endpoint returns JSON:

```json
{
  "job_id": "...",
  "url": "https://example.com",
  "status": "processing",
  "error": ""
}
```

When the job completes, the same endpoint returns `application/pdf` bytes with an attachment filename.

### Dashboard and realtime updates

Open `http://localhost:8080/` to submit jobs and watch the queue update live. Connect to `ws://localhost:8080/ws/jobs` to receive full queue snapshots whenever a job changes state.

Example websocket message:

```json
{
  "type": "jobs",
  "jobs": [
    {
      "id": "...",
      "url": "https://example.com",
      "status": "completed",
      "created_at": "2026-05-01T09:18:54Z",
      "updated_at": "2026-05-01T09:18:56Z"
    }
  ]
}
```

### Operations endpoints

| Endpoint | Purpose |
| --- | --- |
| `GET /healthz` | Liveness probe. Returns `{"status":"ok"}`. |
| `GET /readyz` | Readiness probe. Returns `{"status":"ready"}`. |
| `GET /version` | Returns the build version injected by CI/CD. |
| `GET /metrics` | Prometheus text exposition metrics for HTTP requests, queue depth, worker count, job statuses, render durations, and PDF bytes. |
| `GET /openapi.yaml` | OpenAPI 3.0 YAML document for the public HTTP API. |

Metrics include counters and gauges such as `chromaflow_jobs_submitted_total`, `chromaflow_jobs_rejected_total`, `chromaflow_queue_depth`, `chromaflow_active_workers`, `chromaflow_jobs_in_storage`, `chromaflow_pdf_render_duration_seconds`, `chromaflow_pdf_bytes_total`, and HTTP request metrics.

ChromaFlow logs are structured JSON on stdout. The log records include stable fields such as `service`, `version`, `level`, `msg`, `job_id`, `worker_id`, `url_host`, `duration`, `pdf_bytes`, and `error` where applicable.

## Configuration

Environment variables:

| Name | Default | Description |
| --- | --- | --- |
| `PORT` | `8080` | HTTP server port. |
| `NUM_WORKERS` | `0` | Number of workers. `0` auto-detects as `runtime.NumCPU() * 2`. |
| `QUEUE_SIZE` | `100` | In-memory queue buffer size. |
| `PAGE_TIMEOUT` | `30` | Per-page render timeout in seconds. |
| `RESULT_TTL` | `3600` | Reserved for future result expiration. |
| `CHROME_WS_URL` | empty | Existing Chrome DevTools websocket URL. Empty launches local Chromium. |
| `CHROME_BIN` | auto-detected | Chromium/Chrome executable path when launching a local browser. |

## Run locally as a binary

Requirements:

- Go 1.24+
- Chromium or Chrome installed on the host, or a `CHROME_WS_URL` pointing to a running browser

```sh
GOCACHE=/tmp/chromaflow-gocache go test ./...
GOCACHE=/tmp/chromaflow-gocache go build -trimpath -o dist/chromaflow ./cmd/server
PORT=8080 NUM_WORKERS=2 PAGE_TIMEOUT=30 ./dist/chromaflow
```

On Windows, build and run the `.exe` variant:

```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -trimpath -o dist/chromaflow.exe ./cmd/server
$env:PORT="8080"; .\dist\chromaflow.exe
```

Make sure Chrome or Chromium is installed and set `CHROME_BIN` if it is not discoverable from `PATH`.

## Run with Docker

```sh
docker build -t chromaflow .
docker run --rm -p 8080:8080 \
  -e NUM_WORKERS=4 \
  -e QUEUE_SIZE=100 \
  -e PAGE_TIMEOUT=30 \
  chromaflow
```

Or use Compose, which also starts Prometheus, Grafana, Loki, and Promtail:

```sh
docker compose up --build
```

Local service URLs:

| URL | Service |
| --- | --- |
| `http://localhost:8080` | ChromaFlow dashboard/API. |
| `http://localhost:8080/metrics` | ChromaFlow metrics scraped by Prometheus. |
| `http://localhost:9090` | Prometheus. |
| `http://localhost:3000` | Grafana (`admin` / `admin` locally). The bundled dashboard uses Prometheus for metrics and Loki for logs. |
| `http://localhost:3100` | Loki API. |

Released images are published by GitHub Actions to GitHub Container Registry as:

```sh
docker pull ghcr.io/<owner>/<repo>:<tag>
```

For this repository, replace `<owner>/<repo>` with the GitHub repository path after it is published.

## Kubernetes

Plain manifests live in `k8s/` and deploy ChromaFlow plus Prometheus, Grafana, Loki, and Promtail into a `chromaflow` namespace. Replace the image placeholder in `k8s/chromaflow.yaml` before deploying:

```sh
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/
```

Ingress examples use `chromaflow.local` for the app and `grafana.chromaflow.local` for Grafana with an `nginx` ingress class. Production clusters should add TLS, persistent volumes for Prometheus/Loki/Grafana, real secrets, resource tuning, and environment-specific ingress annotations.

## Future dependencies and auth

The current single-node deployment keeps queue and PDF bytes in memory. The next scale-out design should add Redis for shared queue/result coordination, object storage such as MinIO for generated PDFs, optional RabbitMQ or Kafka for asynchronous job events, and token-based authentication before the API is exposed outside trusted networks. When these are added, update the OpenAPI security schemes, Kubernetes secrets/config maps, Docker Compose services, and operational docs together.

## CI/CD and releases

This repository includes two GitHub Actions workflows:

- **CI** (`.github/workflows/ci.yml`) runs formatting, `go vet`, `go test -race ./...`, a Linux binary build, and a Docker image build on pushes and pull requests.
- **Release** (`.github/workflows/release.yml`) runs on tags matching `v*.*.*` and publishes:
  - Linux `amd64` and `arm64` binaries.
  - Windows `amd64` and `arm64` binaries.
  - SHA-256 checksum files.
  - Multi-architecture container images to `ghcr.io/${{ github.repository }}`.
  - A GitHub Release with generated release notes.

Release procedure:

```sh
git tag v0.1.0
git push origin v0.1.0
```

## Production notes

- Treat submitted URLs as untrusted input. ChromaFlow currently restricts schemes to `http` and `https`, but production deployments should also add SSRF defenses for private networks, redirects, DNS rebinding, metadata endpoints, and internal-only hostnames before exposing the service publicly.
- Jobs and PDFs are stored in memory. Restarts lose in-flight work and completed PDFs. Redis, MinIO, and optional message-broker integration are planned future changes, not active runtime dependencies yet.
- Use resource limits around containers because Chromium can consume significant CPU and memory.
- Keep `PAGE_TIMEOUT`, `QUEUE_SIZE`, and `NUM_WORKERS` aligned with host capacity.
- Prefer the container image for consistent Chromium dependencies. Binary deployments must install and maintain Chrome/Chromium separately.

## Roadmap

Near-term production hardening is tracked in `IMPROVEMENTS.md`. The main scalability milestone is a Redis-backed shared queue/result backend so multiple ChromaFlow instances can process jobs horizontally.

## License

ChromaFlow is licensed under the MIT License. See `LICENSE`.
