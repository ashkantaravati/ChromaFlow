# AGENTS.md

## Project Overview

ChromaFlow is a Go HTTP service that converts web pages to PDFs. It accepts URL jobs through an API/dashboard, stores jobs in an in-memory queue, renders pages with headless Chromium via go-rod, and serves either status JSON or the completed PDF.

The current architecture is single-node and intentionally simple. Horizontal scalability is planned through a future Redis-backed shared queue/result layer.

## Common Commands

- Format: `gofmt -w <files>`
- Build/test all packages: `GOCACHE=/tmp/chromaflow-gocache go test ./...`
- Vet all packages: `GOCACHE=/tmp/chromaflow-gocache go vet ./...`
- Run locally: `PORT=8080 NUM_WORKERS=2 PAGE_TIMEOUT=30 go run ./cmd/server`
- Build local binary: `GOCACHE=/tmp/chromaflow-gocache CGO_ENABLED=0 go build -trimpath -o dist/chromaflow ./cmd/server`
- Build Docker image: `docker build -t chromaflow .`
- Run Docker image: `docker run --rm -p 8080:8080 chromaflow`

Use `/tmp/chromaflow-gocache` for Go commands when the default Go cache may not be writable in a sandbox.

## Code Map

- `cmd/server/main.go`: application wiring, HTTP routes, worker startup, graceful shutdown, and version endpoint.
- `internal/api/handler.go`: dashboard page, `POST /pdf`, `GET /pdf/{id}`, health/readiness endpoints, and `GET /ws/jobs` websocket handler.
- `internal/config/config.go`: environment configuration.
- `internal/pdf/generator.go`: Chromium launch/connect and PDF rendering.
- `internal/queue/`: in-memory job queue and job/result types.
- `internal/realtime/`: small stdlib websocket hub for broadcasting queue snapshots.
- `internal/storage/`: in-memory result storage and job snapshot listing.
- `internal/worker/`: worker pool and job processing.
- `.github/workflows/`: CI and release automation.

## Commit Guidelines

Use semantic commit messages in the form `type(scope): short imperative summary`, for example:

- `feat(api): add health endpoints`
- `fix(queue): report full queue without blocking`
- `ci(release): publish chromaflow binaries`

## Development Guidelines

- Preserve the simple v0 architecture unless the task explicitly asks for persistence or a larger redesign.
- Avoid `Must*` Rod calls in production paths; return errors so failed renders do not crash the service.
- Keep per-job timeout/cancellation behavior intact when editing PDF generation.
- Be careful with arbitrary URLs. Changes that broaden URL support must consider SSRF, local file access, redirects, private network targets, and metadata services.
- Do not introduce persistent storage, authentication, Redis, or external queues without updating `README.md` and `IMPROVEMENTS.md`.
- Add or update tests for queue behavior, handler status transitions, websocket snapshots, and PDF generation abstractions when changing those areas.
- If a change affects the dashboard UI, take a screenshot when feasible.
