package queue

import "errors"

var ErrQueueFull = errors.New("queue is full")

type MemoryQueue struct {
	jobs chan Job
}

func NewMemoryQueue(size int) *MemoryQueue {
	return &MemoryQueue{
		jobs: make(chan Job, size),
	}
}

func (q *MemoryQueue) Push(job Job) error {
	select {
	case q.jobs <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

func (q *MemoryQueue) Pop() <-chan Job {
	return q.jobs
}

func (q *MemoryQueue) Len() int {
	return len(q.jobs)
}

func (q *MemoryQueue) Cap() int {
	return cap(q.jobs)
}
