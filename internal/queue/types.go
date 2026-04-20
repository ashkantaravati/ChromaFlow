package queue

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
	Status JobStatus
	PDF    []byte // PDF bytes
	Error  string
}
