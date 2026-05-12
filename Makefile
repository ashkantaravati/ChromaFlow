GO ?= go
GOCACHE ?= /tmp/chromaflow-gocache
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: test vet build run docker-build compose-up k8s-apply release-snapshot clean

test:
	GOCACHE=$(GOCACHE) $(GO) test ./...

vet:
	GOCACHE=$(GOCACHE) $(GO) vet ./...

build:
	mkdir -p dist
	GOCACHE=$(GOCACHE) CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/chromaflow ./cmd/server

run:
	GOCACHE=$(GOCACHE) PORT=$${PORT:-8080} NUM_WORKERS=$${NUM_WORKERS:-2} PAGE_TIMEOUT=$${PAGE_TIMEOUT:-30} $(GO) run ./cmd/server

docker-build:
	docker build -t chromaflow .

compose-up:
	docker compose up --build

k8s-apply:
	kubectl apply -f k8s/namespace.yaml
	kubectl apply -f k8s/

release-snapshot:
	mkdir -p dist
	GOCACHE=$(GOCACHE) CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/chromaflow-linux-amd64 ./cmd/server
	GOCACHE=$(GOCACHE) CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/chromaflow-linux-arm64 ./cmd/server
	GOCACHE=$(GOCACHE) CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/chromaflow-windows-amd64.exe ./cmd/server
	GOCACHE=$(GOCACHE) CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/chromaflow-windows-arm64.exe ./cmd/server

clean:
	rm -rf dist
