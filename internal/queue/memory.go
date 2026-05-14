package queue

import "context"

type MemoryQueue struct {
	jobs chan Job
}

func NewMemoryQueue(size int) *MemoryQueue {
	return &MemoryQueue{jobs: make(chan Job, size)}
}

func (q *MemoryQueue) Push(ctx context.Context, job Job) error {
	select {
	case q.jobs <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

func (q *MemoryQueue) Pop(ctx context.Context) (Job, error) {
	select {
	case <-ctx.Done():
		return Job{}, ctx.Err()
	case job := <-q.jobs:
		return job, nil
	}
}

func (q *MemoryQueue) Ack(ctx context.Context, job Job) error { return nil }

func (q *MemoryQueue) PopChan() <-chan Job { return q.jobs }

func (q *MemoryQueue) Len(ctx context.Context) (int, error) { return len(q.jobs), nil }

func (q *MemoryQueue) Cap() int { return cap(q.jobs) }
