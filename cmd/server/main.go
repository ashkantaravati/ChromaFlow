package main

import (
	"chromaflow/internal/api"
	"chromaflow/internal/config"
	"chromaflow/internal/pdf"
	"chromaflow/internal/queue"
	"chromaflow/internal/storage"
	"chromaflow/internal/worker"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg := config.Load()
	log.Printf("Starting chromaflow v0 with %d workers", cfg.NumWorkers)

	// Initialize components
	q := queue.NewMemoryQueue(cfg.QueueSize)
	s := storage.NewMemoryStorage()
	g := pdf.NewGenerator(cfg.PageTimeout)
	pool := worker.NewPool(q, s, g, cfg.NumWorkers)

	// Start worker pool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	// Setup HTTP routes
	handler := api.NewHandler(q, s)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /pdf", handler.SubmitJob)
	mux.HandleFunc("GET /pdf/{id}", handler.GetJob)

	// Start server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	go func() {
		log.Printf("Server listening on port %d", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	cancel() // Stop workers

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
