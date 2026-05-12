package worker

import (
	"chromaflow/internal/observability"
	"chromaflow/internal/pdf"
	"chromaflow/internal/queue"
	"chromaflow/internal/storage"
	"context"
	"log/slog"
	"net/url"
	"time"
)

type Pool struct {
	queue      *queue.MemoryQueue
	storage    *storage.MemoryStorage
	generator  *pdf.Generator
	numWorkers int
	metrics    *observability.Metrics
	logger     *slog.Logger
}

func NewPool(q *queue.MemoryQueue, s *storage.MemoryStorage, g *pdf.Generator, numWorkers int, metrics *observability.Metrics, logger *slog.Logger) *Pool {
	if logger == nil {
		logger = slog.Default()
	}
	return &Pool{
		queue:      q,
		storage:    s,
		generator:  g,
		numWorkers: numWorkers,
		metrics:    metrics,
		logger:     logger,
	}
}

func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.numWorkers; i++ {
		go p.worker(ctx, i)
	}
}

func (p *Pool) worker(ctx context.Context, id int) {
	logger := p.logger.With(slog.Int("worker_id", id))
	logger.Info("worker started")
	if p.metrics != nil {
		p.metrics.WorkerStarted()
		defer p.metrics.WorkerStopped()
	}
	for {
		select {
		case <-ctx.Done():
			logger.Info("worker stopped")
			return
		case job := <-p.queue.Pop():
			p.processJob(ctx, id, job)
		}
	}
}

func (p *Pool) processJob(ctx context.Context, workerID int, job queue.Job) {
	start := time.Now()
	logger := p.logger.With(
		slog.String("job_id", job.ID),
		slog.Int("worker_id", workerID),
		slog.String("url_host", hostForLog(job.URL)),
	)
	logger.Info("job processing started")

	p.storage.Set(job.ID, &queue.JobResult{URL: job.URL, Status: queue.StatusProcessing})

	pdfBytes, err := p.generator.GeneratePDF(ctx, job.URL)
	if err != nil {
		duration := time.Since(start)
		logger.Error("job failed", slog.Duration("duration", duration), slog.String("error", err.Error()))
		if p.metrics != nil {
			p.metrics.RenderFinished(string(queue.StatusFailed), duration, 0)
		}
		p.storage.Set(job.ID, &queue.JobResult{
			URL:    job.URL,
			Status: queue.StatusFailed,
			Error:  err.Error(),
		})
		return
	}

	duration := time.Since(start)
	p.storage.Set(job.ID, &queue.JobResult{
		URL:    job.URL,
		Status: queue.StatusCompleted,
		PDF:    pdfBytes,
	})
	if p.metrics != nil {
		p.metrics.RenderFinished(string(queue.StatusCompleted), duration, len(pdfBytes))
	}
	logger.Info("job completed", slog.Duration("duration", duration), slog.Int("pdf_bytes", len(pdfBytes)))
}

func hostForLog(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
