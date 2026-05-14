package worker

import (
	"chromaflow/internal/blob"
	"chromaflow/internal/cancellation"
	"chromaflow/internal/observability"
	"chromaflow/internal/pdf"
	"chromaflow/internal/queue"
	"chromaflow/internal/storage"
	"chromaflow/internal/webhook"
	"context"
	"errors"
	"log/slog"
	"net/url"
	"sync"
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
	cancels    *cancellation.Registry
	notifier   *webhook.Notifier
	options    Options
	wg         sync.WaitGroup
}

type Options struct {
	RenderMaxRetries int
	RetryBackoff     time.Duration
}

func NewPool(q queue.Backend, s storage.Store, b blob.Store, g *pdf.Generator, numWorkers int, metrics *observability.Metrics, logger *slog.Logger) *Pool {
	return NewPoolWithOptions(q, s, b, g, numWorkers, metrics, logger, nil, nil, Options{})
}

func NewPoolWithOptions(q queue.Backend, s storage.Store, b blob.Store, g *pdf.Generator, numWorkers int, metrics *observability.Metrics, logger *slog.Logger, cancels *cancellation.Registry, notifier *webhook.Notifier, options Options) *Pool {
	if logger == nil {
		logger = slog.Default()
	}
	if cancels == nil {
		cancels = cancellation.NewRegistry()
	}
	if options.RenderMaxRetries < 0 {
		options.RenderMaxRetries = 0
	}
	if options.RetryBackoff <= 0 {
		options.RetryBackoff = 500 * time.Millisecond
	}
	return &Pool{queue: q, storage: s, blobStore: b, generator: g, numWorkers: numWorkers, metrics: metrics, logger: logger, cancels: cancels, notifier: notifier, options: options}
}

func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.numWorkers; i++ {
		p.wg.Add(1)
		go func(workerID int) {
			defer p.wg.Done()
			p.worker(ctx, workerID)
		}(i)
	}
}

func (p *Pool) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
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
		if err := p.queue.Ack(context.WithoutCancel(ctx), job); err != nil {
			logger.Error("job ack failed", slog.String("job_id", job.ID), slog.String("error", err.Error()), slog.String("request_id", job.RequestID))
		}
	}
}

func (p *Pool) processJob(ctx context.Context, workerID int, job queue.Job) {
	start := time.Now()
	logger := p.logger.With(slog.String("job_id", job.ID), slog.Int("worker_id", workerID), slog.String("url_host", hostForLog(job.URL)), slog.String("request_id", job.RequestID))
	logger.Info("job processing started")
	updateCtx := context.WithoutCancel(ctx)

	current, err := p.storage.Get(updateCtx, job.ID)
	if err == nil && current.Status == queue.StatusCanceled {
		logger.Info("job skipped because it was canceled before processing")
		return
	}

	_ = p.storage.Set(updateCtx, job.ID, &queue.JobResult{URL: job.URL, Status: queue.StatusProcessing, IdempotencyKey: job.IdempotencyKey, CallbackURL: job.CallbackURL, RequestID: job.RequestID})

	jobCtx, cancel := context.WithCancel(ctx)
	unregister := p.cancels.Register(job.ID, cancel)
	defer unregister()
	defer cancel()

	pdfBytes, err := p.generateWithRetries(jobCtx, job.URL, logger)
	if err != nil {
		duration := time.Since(start)
		if current, getErr := p.storage.Get(updateCtx, job.ID); getErr == nil && current.Status == queue.StatusCanceled {
			logger.Info("job canceled", slog.Duration("duration", duration))
			if p.metrics != nil {
				p.metrics.RenderFinished(string(queue.StatusCanceled), duration, 0)
			}
			p.notify(updateCtx, job, current)
			return
		}
		statusErr := err.Error()
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			statusErr = "job interrupted by shutdown"
		}
		logger.Error("job failed", slog.Duration("duration", duration), slog.String("error", statusErr))
		if p.metrics != nil {
			p.metrics.RenderFinished(string(queue.StatusFailed), duration, 0)
		}
		result := &queue.JobResult{URL: job.URL, Status: queue.StatusFailed, Error: statusErr, IdempotencyKey: job.IdempotencyKey, CallbackURL: job.CallbackURL, RequestID: job.RequestID}
		_ = p.storage.Set(updateCtx, job.ID, result)
		p.notify(updateCtx, job, result)
		return
	}

	current, err = p.storage.Get(updateCtx, job.ID)
	if err == nil && current.Status == queue.StatusCanceled {
		logger.Info("job rendered but result discarded because it was canceled")
		if p.metrics != nil {
			p.metrics.RenderFinished(string(queue.StatusCanceled), time.Since(start), 0)
		}
		p.notify(updateCtx, job, current)
		return
	}

	pdfKey := "pdf/" + job.ID + ".pdf"
	if err := p.blobStore.Put(updateCtx, pdfKey, pdfBytes); err != nil {
		duration := time.Since(start)
		logger.Error("job storage failed", slog.Duration("duration", duration), slog.String("error", err.Error()))
		if p.metrics != nil {
			p.metrics.RenderFinished(string(queue.StatusFailed), duration, 0)
		}
		result := &queue.JobResult{URL: job.URL, Status: queue.StatusFailed, Error: err.Error(), IdempotencyKey: job.IdempotencyKey, CallbackURL: job.CallbackURL, RequestID: job.RequestID}
		_ = p.storage.Set(updateCtx, job.ID, result)
		p.notify(updateCtx, job, result)
		return
	}

	duration := time.Since(start)
	result := &queue.JobResult{URL: job.URL, Status: queue.StatusCompleted, PDFKey: pdfKey, PDFSize: len(pdfBytes), IdempotencyKey: job.IdempotencyKey, CallbackURL: job.CallbackURL, RequestID: job.RequestID}
	_ = p.storage.Set(updateCtx, job.ID, result)
	if p.metrics != nil {
		p.metrics.RenderFinished(string(queue.StatusCompleted), duration, len(pdfBytes))
	}
	p.notify(updateCtx, job, result)
	logger.Info("job completed", slog.Duration("duration", duration), slog.Int("pdf_bytes", len(pdfBytes)))
}

func (p *Pool) generateWithRetries(ctx context.Context, rawURL string, logger *slog.Logger) ([]byte, error) {
	attempts := p.options.RenderMaxRetries + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		pdfBytes, err := p.generator.GeneratePDF(ctx, rawURL)
		if err == nil {
			return pdfBytes, nil
		}
		lastErr = err
		if ctx.Err() != nil || attempt == attempts {
			break
		}
		logger.Warn("render attempt failed; retrying", slog.Int("attempt", attempt), slog.Int("max_attempts", attempts), slog.String("error", err.Error()))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(p.options.RetryBackoff):
		}
	}
	return nil, lastErr
}

func (p *Pool) notify(ctx context.Context, job queue.Job, result *queue.JobResult) {
	callbackURL := result.CallbackURL
	if callbackURL == "" {
		callbackURL = job.CallbackURL
	}
	if callbackURL == "" || p.notifier == nil {
		return
	}
	if result.Status != queue.StatusCompleted && result.Status != queue.StatusFailed {
		return
	}
	err := p.notifier.Notify(ctx, callbackURL, webhook.Event{JobID: job.ID, URL: result.URL, Status: result.Status, Error: result.Error, PDFKey: result.PDFKey, PDFSize: result.PDFSize, RequestID: result.RequestID})
	if err != nil {
		p.logger.Warn("webhook callback failed", slog.String("job_id", job.ID), slog.String("error", err.Error()), slog.String("request_id", result.RequestID))
	}
}

func hostForLog(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
