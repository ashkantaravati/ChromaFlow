package worker

import (
	"chromaflow/internal/pdf"
	"chromaflow/internal/queue"
	"chromaflow/internal/storage"
	"context"
	"log"
)

type Pool struct {
	queue      *queue.MemoryQueue
	storage    *storage.MemoryStorage
	generator  *pdf.Generator
	numWorkers int
}

func NewPool(q *queue.MemoryQueue, s *storage.MemoryStorage, g *pdf.Generator, numWorkers int) *Pool {
	return &Pool{
		queue:      q,
		storage:    s,
		generator:  g,
		numWorkers: numWorkers,
	}
}

func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.numWorkers; i++ {
		go p.worker(ctx, i)
	}
}

func (p *Pool) worker(ctx context.Context, id int) {
	log.Printf("Worker %d started", id)
	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d stopped", id)
			return
		case job := <-p.queue.Pop():
			p.processJob(ctx, job)
		}
	}
}

func (p *Pool) processJob(ctx context.Context, job queue.Job) {
	log.Printf("Processing job %s: %s", job.ID, job.URL)

	// Update status to processing
	p.storage.Set(job.ID, &queue.JobResult{URL: job.URL, Status: queue.StatusProcessing})

	// Generate PDF
	pdfBytes, err := p.generator.GeneratePDF(ctx, job.URL)
	if err != nil {
		log.Printf("Job %s failed: %v", job.ID, err)
		p.storage.Set(job.ID, &queue.JobResult{
			URL:    job.URL,
			Status: queue.StatusFailed,
			Error:  err.Error(),
		})
		return
	}

	// Store result
	p.storage.Set(job.ID, &queue.JobResult{
		URL:    job.URL,
		Status: queue.StatusCompleted,
		PDF:    pdfBytes,
	})
	log.Printf("Job %s completed (%d bytes)", job.ID, len(pdfBytes))
}
