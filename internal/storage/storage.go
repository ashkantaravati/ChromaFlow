package storage

import (
	"context"

	"chromaflow/internal/observability"
	"chromaflow/internal/queue"
)

type Store interface {
	SetOnChange(func())
	Set(ctx context.Context, jobID string, result *queue.JobResult) error
	Delete(ctx context.Context, jobID string) error
	Get(ctx context.Context, jobID string) (*queue.JobResult, error)
	Cancel(ctx context.Context, jobID string) (*queue.JobResult, error)
	ReserveIdempotencyKey(ctx context.Context, key, jobID string) (string, bool, error)
	Stats(ctx context.Context) observability.StorageStats
	List(ctx context.Context) []queue.JobSnapshot
}
