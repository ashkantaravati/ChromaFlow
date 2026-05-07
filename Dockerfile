# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.24-alpine3.22 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/chromaflow ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache \
    ca-certificates \
    chromium \
    freetype \
    harfbuzz \
    nss \
    ttf-freefont && \
    addgroup -S chromaflow && \
    adduser -S -G chromaflow chromaflow

ENV CHROME_BIN=/usr/bin/chromium-browser \
    PORT=8080

WORKDIR /app
COPY --from=builder /out/chromaflow /app/chromaflow
RUN chown -R chromaflow:chromaflow /app

USER chromaflow
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:${PORT}/healthz >/dev/null || exit 1

CMD ["/app/chromaflow"]
