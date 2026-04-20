package queue

type MemoryQueue struct {
	jobs chan Job
}

func NewMemoryQueue(size int) *MemoryQueue {
	return &MemoryQueue{
		jobs: make(chan Job, size),
	}
}

func (q *MemoryQueue) Push(job Job) error {
	q.jobs <- job
	return nil
}

func (q *MemoryQueue) Pop() <-chan Job {
	return q.jobs
}
