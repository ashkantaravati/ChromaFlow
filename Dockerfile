FROM golang:1.24-alpine AS builder
ENV GOPROXY=https://mirror-go.runflare.com

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o chromaflow ./cmd/server

FROM alpine:latest

# Configure Arvan Cloud mirror
RUN echo "https://mirror.arvancloud.ir/alpine/v3.23/main" > /etc/apk/repositories && \
    echo "https://mirror.arvancloud.ir/alpine/v3.23/community" >> /etc/apk/repositories

RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont

ENV CHROME_BIN=/usr/bin/chromium-browser

WORKDIR /app
COPY --from=builder /app/chromaflow .

EXPOSE 8080
CMD ["./chromaflow"]
