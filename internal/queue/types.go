package queue

import (
	"context"
	"errors"
	"time"
)

var (
	ErrQueueFull       = errors.New("queue is full")
	ErrDuplicateJob    = errors.New("duplicate idempotency key")
	ErrJobNotAvailable = errors.New("job not available")
)

type Backend interface {
	Push(ctx context.Context, job Job) error
	Pop(ctx context.Context) (Job, error)
	Ack(ctx context.Context, job Job) error
	Len(ctx context.Context) (int, error)
	Cap() int
}

type Job struct {
	ID             string
	URL            string
	IdempotencyKey string
	QueueMessageID string
}

type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
	StatusCanceled   JobStatus = "canceled"
)

type JobResult struct {
	URL            string
	Status         JobStatus
	PDF            []byte // Used only by the in-memory blob backend for backwards-compatible tests.
	PDFKey         string
	PDFSize        int
	Error          string
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type JobSnapshot struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Status    JobStatus `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
