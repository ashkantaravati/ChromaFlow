package main

import (
	"chromaflow/internal/api"
	"chromaflow/internal/blob"
	"chromaflow/internal/cancellation"
	"chromaflow/internal/config"
	"chromaflow/internal/observability"
	"chromaflow/internal/pdf"
	"chromaflow/internal/queue"
	"chromaflow/internal/realtime"
	"chromaflow/internal/storage"
	"chromaflow/internal/webhook"
	"chromaflow/internal/worker"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	cfg := config.Load()
	logger := observability.NewLogger("chromaflow", version, cfg.LogLevel)
	logger.Info("starting chromaflow", slog.Int("workers", cfg.NumWorkers), slog.String("queue_backend", cfg.QueueBackend), slog.String("storage_backend", cfg.StorageBackend), slog.String("blob_backend", cfg.BlobBackend))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q, err := buildQueue(ctx, cfg)
	if err != nil {
		logger.Error("queue setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	s, err := buildStorage(cfg)
	if err != nil {
		logger.Error("storage setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	blobs, err := buildBlobStore(ctx, cfg)
	if err != nil {
		logger.Error("blob storage setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	hub := realtime.NewHub()
	metrics := observability.NewMetrics()
	metrics.SetQueueStats(func() (int, int) {
		depth, err := q.Len(context.Background())
		if err != nil {
			return 0, q.Cap()
		}
		return depth, q.Cap()
	})
	metrics.SetStorageStats(func() observability.StorageStats { return s.Stats(context.Background()) })
	s.SetOnChange(func() { hub.BroadcastJobs(s.List(context.Background())) })
	cancels := cancellation.NewRegistry()
	notifier := webhook.NewNotifier(time.Duration(cfg.WebhookTimeout) * time.Second)
	g := pdf.NewGenerator(cfg.PageTimeout, cfg.ChromeWSURL)
	defer g.Close()
	pool := worker.NewPoolWithOptions(q, s, blobs, g, cfg.NumWorkers, metrics, logger, cancels, notifier, worker.Options{RenderMaxRetries: cfg.RenderMaxRetries, RetryBackoff: time.Duration(cfg.RenderRetryBackoffMS) * time.Millisecond})
	pool.Start(ctx)

	handler := api.NewHandlerWithOptions(q, s, blobs, hub, metrics, logger, cancels, api.HandlerOptions{RequireIdempotencyKey: cfg.RequireIdempotencyKey, DefaultWebhookURL: cfg.WebhookURL})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handler.Dashboard)
	mux.HandleFunc("POST /pdf", handler.SubmitJob)
	mux.HandleFunc("GET /pdf/{id}", handler.GetJob)
	mux.HandleFunc("DELETE /pdf/{id}", handler.CancelJob)
	mux.HandleFunc("POST /pdf/{id}/cancel", handler.CancelJob)
	mux.HandleFunc("GET /ws/jobs", handler.JobsWebSocket)
	mux.HandleFunc("GET /healthz", handler.Healthz)
	mux.HandleFunc("GET /readyz", handler.Readyz)
	mux.HandleFunc("GET /openapi.yaml", handler.OpenAPI)
	mux.Handle("GET /metrics", metrics.Handler())
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":%q}`+"\n", version)
	})

	server := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: metrics.Middleware(observability.RequestMiddleware(mux))}

	go func() {
		logger.Info("server listening", slog.Int("port", cfg.Port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	cancel()
	if err := pool.Wait(shutdownCtx); err != nil {
		logger.Warn("worker shutdown did not complete before timeout", slog.String("error", err.Error()))
	}
	logger.Info("server stopped")
}

func buildQueue(ctx context.Context, cfg *config.Config) (queue.Backend, error) {
	switch strings.ToLower(cfg.QueueBackend) {
	case "", "memory":
		return queue.NewMemoryQueue(cfg.QueueSize), nil
	case "redis":
		return queue.NewRedisQueue(ctx, cfg.RedisURL, cfg.RedisKeyPrefix, cfg.QueueSize, time.Duration(cfg.RedisVisibilityTimeout)*time.Second)
	default:
		return nil, fmt.Errorf("unsupported QUEUE_BACKEND %q", cfg.QueueBackend)
	}
}

func buildStorage(cfg *config.Config) (storage.Store, error) {
	backend := cfg.StorageBackend
	if backend == "" {
		backend = cfg.QueueBackend
	}
	switch strings.ToLower(backend) {
	case "", "memory":
		return storage.NewMemoryStorage(), nil
	case "redis":
		return storage.NewRedisStorage(cfg.RedisURL, cfg.RedisKeyPrefix)
	default:
		return nil, fmt.Errorf("unsupported STORAGE_BACKEND %q", cfg.StorageBackend)
	}
}

func buildBlobStore(ctx context.Context, cfg *config.Config) (blob.Store, error) {
	switch strings.ToLower(cfg.BlobBackend) {
	case "", "memory":
		return blob.NewMemoryStore(), nil
	case "s3", "minio":
		return blob.NewS3Store(ctx, blob.S3Config{Endpoint: cfg.S3Endpoint, AccessKeyID: cfg.S3AccessKeyID, SecretAccessKey: cfg.S3SecretAccessKey, Bucket: cfg.S3Bucket, Region: cfg.S3Region, UseSSL: cfg.S3UseSSL})
	default:
		return nil, fmt.Errorf("unsupported BLOB_BACKEND %q", cfg.BlobBackend)
	}
}
