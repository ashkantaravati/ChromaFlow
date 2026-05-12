package main

import (
	"chromaflow/internal/api"
	"chromaflow/internal/config"
	"chromaflow/internal/observability"
	"chromaflow/internal/pdf"
	"chromaflow/internal/queue"
	"chromaflow/internal/realtime"
	"chromaflow/internal/storage"
	"chromaflow/internal/worker"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	logger := observability.NewLogger("chromaflow", version)
	cfg := config.Load()
	logger.Info("starting chromaflow", slog.Int("workers", cfg.NumWorkers))

	q := queue.NewMemoryQueue(cfg.QueueSize)
	s := storage.NewMemoryStorage()
	hub := realtime.NewHub()
	metrics := observability.NewMetrics()
	metrics.SetQueueStats(func() (int, int) { return q.Len(), q.Cap() })
	metrics.SetStorageStats(s.Stats)
	s.SetOnChange(func() { hub.BroadcastJobs(s.List()) })
	g := pdf.NewGenerator(cfg.PageTimeout, cfg.ChromeWSURL)
	pool := worker.NewPool(q, s, g, cfg.NumWorkers, metrics, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	handler := api.NewHandler(q, s, hub, metrics, logger)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handler.Dashboard)
	mux.HandleFunc("POST /pdf", handler.SubmitJob)
	mux.HandleFunc("GET /pdf/{id}", handler.GetJob)
	mux.HandleFunc("GET /ws/jobs", handler.JobsWebSocket)
	mux.HandleFunc("GET /healthz", handler.Healthz)
	mux.HandleFunc("GET /readyz", handler.Readyz)
	mux.HandleFunc("GET /openapi.yaml", handler.OpenAPI)
	mux.Handle("GET /metrics", metrics.Handler())
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":%q}`+"\n", version)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: metrics.Middleware(mux),
	}

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
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("server stopped")
}
