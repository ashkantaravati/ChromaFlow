package api

import (
	"chromaflow/internal/queue"
	"chromaflow/internal/storage"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

type Handler struct {
	queue   *queue.MemoryQueue
	storage *storage.MemoryStorage
}

func NewHandler(q *queue.MemoryQueue, s *storage.MemoryStorage) *Handler {
	return &Handler{queue: q, storage: s}
}

type SubmitRequest struct {
	URL string `json:"url"`
}

type SubmitResponse struct {
	JobID     string `json:"job_id"`
	StatusURL string `json:"status_url"`
}

func (h *Handler) SubmitJob(w http.ResponseWriter, r *http.Request) {
	var req SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	jobID := uuid.New().String()
	job := queue.Job{ID: jobID, URL: req.URL}

	// Initialize job as pending
	h.storage.Set(jobID, &queue.JobResult{Status: queue.StatusPending})

	// Push to queue
	if err := h.queue.Push(job); err != nil {
		http.Error(w, "Failed to queue job", http.StatusInternalServerError)
		return
	}

	resp := SubmitResponse{
		JobID:     jobID,
		StatusURL: "/pdf/" + jobID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")

	result, err := h.storage.Get(jobID)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	if result.Status == queue.StatusCompleted {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", "attachment; filename="+jobID+".pdf")
		w.Write(result.PDF)
		return
	}

	// Return status as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"job_id": jobID,
		"status": result.Status,
		"error":  result.Error,
	})
}
