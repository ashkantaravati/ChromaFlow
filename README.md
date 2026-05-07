# Chromaflow

Chromaflow is a small Go service that turns web pages into PDF files. Clients submit a URL, Chromaflow queues the job, workers render the page with headless Chromium, and the completed PDF is returned from a status endpoint.

## Features

- HTTP API for submitting URL-to-PDF jobs.
- Browser dashboard at `/` for submitting jobs and watching status updates.
- WebSocket stream at `/ws/jobs` for real-time job snapshots.
- In-memory job queue and result store.
- Configurable worker count, queue size, and page timeout.
- Headless Chromium rendering through [go-rod](https://github.com/go-rod/rod).
- Docker image with Chromium installed.

## Web Interface

Open `http://localhost:8080/` in a browser to submit PDF jobs and watch the current in-memory job queue update in real time. Completed jobs include a download link for the generated PDF.

## API

### Submit a PDF job

```sh
curl -X POST http://localhost:8080/pdf \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com"}'
```

Response:

```json
{
  "job_id": "...",
  "status_url": "/pdf/..."
}
```

### Fetch job status or PDF

```sh
curl -OJ http://localhost:8080/pdf/<job_id>
```

If the job is still pending, processing, or failed, the endpoint returns JSON:

```json
{
  "job_id": "...",
  "status": "processing",
  "error": ""
}
```

When the job completes, the same endpoint returns `application/pdf` bytes with an attachment filename.

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

`CHROME_BIN` can also be set to the Chromium/Chrome executable path when launching a local browser.

## Run Locally

Requirements:

- Go 1.24+
- Chromium or Chrome available on the host, or a `CHROME_WS_URL` pointing to a running browser

```sh
GOCACHE=/tmp/chromaflow-gocache go test ./...
PORT=8080 NUM_WORKERS=2 PAGE_TIMEOUT=30 go run ./cmd/server
```

On Ubuntu, `/usr/bin/chromium-browser` may be only a Snap launcher stub. If local rendering fails with a Snap install message, run Chromaflow with Docker or set `CHROME_BIN` to a real Chromium/Chrome binary.

## Run With Docker

```sh
docker build -t chromaflow .
docker run --rm -p 8080:8080 \
  -e NUM_WORKERS=4 \
  -e QUEUE_SIZE=100 \
  -e PAGE_TIMEOUT=30 \
  chromaflow
```

Or use Compose:

```sh
docker compose up --build
```

## WebSocket Events

Connect to `ws://localhost:8080/ws/jobs` to receive full queue snapshots whenever a job changes state. Messages look like:

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

## Development Notes

The current implementation is intentionally simple and stores queues/results in memory. Restarting the process loses jobs and PDFs. See `IMPROVEMENTS.md` for known limitations and suggested next steps.
