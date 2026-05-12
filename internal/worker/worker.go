package worker

import (
	"chromaflow/internal/blob"
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
	queue      queue.Backend
	storage    storage.Store
	blobStore  blob.Store
	generator  *pdf.Generator
	numWorkers int
	metrics    *observability.Metrics
	logger     *slog.Logger
}

func NewPool(q queue.Backend, s storage.Store, b blob.Store, g *pdf.Generator, numWorkers int, metrics *observability.Metrics, logger *slog.Logger) *Pool {
	if logger == nil {
		logger = slog.Default()
	}
	return &Pool{queue: q, storage: s, blobStore: b, generator: g, numWorkers: numWorkers, metrics: metrics, logger: logger}
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
		job, err := p.queue.Pop(ctx)
		if err != nil {
			if ctx.Err() != nil {
				logger.Info("worker stopped")
				return
			}
			logger.Error("job dequeue failed", slog.String("error", err.Error()))
			time.Sleep(time.Second)
			continue
		}
		p.processJob(ctx, id, job)
		if err := p.queue.Ack(ctx, job); err != nil {
			logger.Error("job ack failed", slog.String("job_id", job.ID), slog.String("error", err.Error()))
		}
	}
}

func (p *Pool) processJob(ctx context.Context, workerID int, job queue.Job) {
	start := time.Now()
	logger := p.logger.With(slog.String("job_id", job.ID), slog.Int("worker_id", workerID), slog.String("url_host", hostForLog(job.URL)))
	logger.Info("job processing started")

	current, err := p.storage.Get(ctx, job.ID)
	if err == nil && current.Status == queue.StatusCanceled {
		logger.Info("job skipped because it was canceled before processing")
		return
	}

	_ = p.storage.Set(ctx, job.ID, &queue.JobResult{URL: job.URL, Status: queue.StatusProcessing, IdempotencyKey: job.IdempotencyKey})

	pdfBytes, err := p.generator.GeneratePDF(ctx, job.URL)
	if err != nil {
		duration := time.Since(start)
		logger.Error("job failed", slog.Duration("duration", duration), slog.String("error", err.Error()))
		if p.metrics != nil {
			p.metrics.RenderFinished(string(queue.StatusFailed), duration, 0)
		}
		_ = p.storage.Set(ctx, job.ID, &queue.JobResult{URL: job.URL, Status: queue.StatusFailed, Error: err.Error(), IdempotencyKey: job.IdempotencyKey})
		return
	}

	current, err = p.storage.Get(ctx, job.ID)
	if err == nil && current.Status == queue.StatusCanceled {
		logger.Info("job rendered but result discarded because it was canceled")
		return
	}

	pdfKey := "pdf/" + job.ID + ".pdf"
	if err := p.blobStore.Put(ctx, pdfKey, pdfBytes); err != nil {
		duration := time.Since(start)
		logger.Error("job storage failed", slog.Duration("duration", duration), slog.String("error", err.Error()))
		if p.metrics != nil {
			p.metrics.RenderFinished(string(queue.StatusFailed), duration, 0)
		}
		_ = p.storage.Set(ctx, job.ID, &queue.JobResult{URL: job.URL, Status: queue.StatusFailed, Error: err.Error(), IdempotencyKey: job.IdempotencyKey})
		return
	}

	duration := time.Since(start)
	_ = p.storage.Set(ctx, job.ID, &queue.JobResult{URL: job.URL, Status: queue.StatusCompleted, PDFKey: pdfKey, PDFSize: len(pdfBytes), IdempotencyKey: job.IdempotencyKey})
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
