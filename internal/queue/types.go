package queue

import "time"

type Job struct {
	ID  string
	URL string
}

type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
)

type JobResult struct {
	URL       string
	Status    JobStatus
	PDF       []byte // PDF bytes
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type JobSnapshot struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Status    JobStatus `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
