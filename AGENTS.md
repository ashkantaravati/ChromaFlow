# AGENTS.md

## Project Overview

Chromaflow is a Go HTTP service for converting web pages to PDF files. It accepts URL jobs, stores them in an in-memory queue, renders pages with headless Chromium via go-rod, and serves either job status JSON or the completed PDF.

## Common Commands

- Build/test all packages: `GOCACHE=/tmp/chromaflow-gocache go test ./...`
- Run locally: `PORT=8080 NUM_WORKERS=2 PAGE_TIMEOUT=30 go run ./cmd/server`
- Build Docker image: `docker build -t chromaflow .`
- Run Docker image: `docker run --rm -p 8080:8080 chromaflow`

Use `/tmp/chromaflow-gocache` for Go commands when the default user Go cache is not writable in the sandbox.

## Code Map

- `cmd/server/main.go`: application wiring, HTTP routes, worker startup, graceful shutdown.
- `internal/api/handler.go`: `POST /pdf` submission and `GET /pdf/{id}` status/PDF responses.
- `internal/config/config.go`: environment configuration.
- `internal/pdf/generator.go`: Chromium launch/connect and PDF rendering.
- `internal/queue/`: in-memory job queue and job/result types.
- `internal/storage/`: in-memory result storage.
- `internal/worker/`: worker pool and job processing.

## Development Guidelines

- Preserve the simple v0 architecture unless the task explicitly asks for persistence or a larger redesign.
- Avoid `Must*` Rod calls in production paths; return errors so failed renders do not crash the service.
- Keep per-job timeout/cancellation behavior intact when editing PDF generation.
- Be careful with arbitrary URLs. Changes that broaden URL support should consider SSRF, local file access, redirects, and private network targets.
- Do not introduce persistent storage, authentication, or external queues without updating `README.md` and `IMPROVEMENTS.md`.
- Add or update tests for queue behavior, handler status transitions, and PDF generation abstractions when changing those areas.
