package api

import (
	"chromaflow/internal/blob"
	"chromaflow/internal/queue"
	"chromaflow/internal/realtime"
	"chromaflow/internal/storage"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubmitJobAcceptsHTTPURL(t *testing.T) {
	h := NewHandler(queue.NewMemoryQueue(1), storage.NewMemoryStorage(), blob.NewMemoryStore(), realtime.NewHub(), nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/pdf", strings.NewReader(`{"url":"https://example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.SubmitJob(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusAccepted, rr.Body.String())
	}

	var resp SubmitResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.JobID == "" || resp.StatusURL != "/pdf/"+resp.JobID {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestSubmitJobRejectsUnsupportedScheme(t *testing.T) {
	h := NewHandler(queue.NewMemoryQueue(1), storage.NewMemoryStorage(), blob.NewMemoryStore(), realtime.NewHub(), nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/pdf", strings.NewReader(`{"url":"file:///etc/passwd"}`))
	rr := httptest.NewRecorder()

	h.SubmitJob(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestSubmitJobReturnsServiceUnavailableWhenQueueFull(t *testing.T) {
	h := NewHandler(queue.NewMemoryQueue(0), storage.NewMemoryStorage(), blob.NewMemoryStore(), realtime.NewHub(), nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/pdf", strings.NewReader(`{"url":"https://example.com"}`))
	rr := httptest.NewRecorder()

	h.SubmitJob(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestHealthz(t *testing.T) {
	h := NewHandler(queue.NewMemoryQueue(1), storage.NewMemoryStorage(), blob.NewMemoryStore(), realtime.NewHub(), nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	h.Healthz(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestSubmitJobReturnsExistingJobForIdempotencyKey(t *testing.T) {
	h := NewHandler(queue.NewMemoryQueue(2), storage.NewMemoryStorage(), blob.NewMemoryStore(), realtime.NewHub(), nil, nil)
	first := httptest.NewRequest(http.MethodPost, "/pdf", strings.NewReader(`{"url":"https://example.com"}`))
	first.Header.Set("Content-Type", "application/json")
	first.Header.Set("Idempotency-Key", "same-key")
	firstRR := httptest.NewRecorder()
	h.SubmitJob(firstRR, first)
	if firstRR.Code != http.StatusAccepted {
		t.Fatalf("first status = %d", firstRR.Code)
	}
	var firstResp SubmitResponse
	if err := json.NewDecoder(firstRR.Body).Decode(&firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	second := httptest.NewRequest(http.MethodPost, "/pdf", strings.NewReader(`{"url":"https://example.com"}`))
	second.Header.Set("Content-Type", "application/json")
	second.Header.Set("Idempotency-Key", "same-key")
	secondRR := httptest.NewRecorder()
	h.SubmitJob(secondRR, second)
	if secondRR.Code != http.StatusAccepted {
		t.Fatalf("second status = %d", secondRR.Code)
	}
	var secondResp SubmitResponse
	if err := json.NewDecoder(secondRR.Body).Decode(&secondResp); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if secondResp.JobID != firstResp.JobID || !secondResp.Idempotent {
		t.Fatalf("unexpected idempotent response: %+v first=%+v", secondResp, firstResp)
	}
}

func TestCancelJobMarksPendingJobCanceled(t *testing.T) {
	s := storage.NewMemoryStorage()
	h := NewHandler(queue.NewMemoryQueue(1), s, blob.NewMemoryStore(), realtime.NewHub(), nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/pdf", strings.NewReader(`{"url":"https://example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.SubmitJob(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d", rr.Code)
	}
	var resp SubmitResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	cancelReq := httptest.NewRequest(http.MethodDelete, "/pdf/"+resp.JobID, nil)
	cancelReq.SetPathValue("id", resp.JobID)
	cancelRR := httptest.NewRecorder()
	h.CancelJob(cancelRR, cancelReq)
	if cancelRR.Code != http.StatusOK {
		t.Fatalf("cancel status = %d", cancelRR.Code)
	}
	result, err := s.Get(cancelReq.Context(), resp.JobID)
	if err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.Status != queue.StatusCanceled {
		t.Fatalf("status = %s, want %s", result.Status, queue.StatusCanceled)
	}
}
