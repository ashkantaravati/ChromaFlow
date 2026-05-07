package api

import (
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
	h := NewHandler(queue.NewMemoryQueue(1), storage.NewMemoryStorage(), realtime.NewHub())
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
	h := NewHandler(queue.NewMemoryQueue(1), storage.NewMemoryStorage(), realtime.NewHub())
	req := httptest.NewRequest(http.MethodPost, "/pdf", strings.NewReader(`{"url":"file:///etc/passwd"}`))
	rr := httptest.NewRecorder()

	h.SubmitJob(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestSubmitJobReturnsServiceUnavailableWhenQueueFull(t *testing.T) {
	h := NewHandler(queue.NewMemoryQueue(0), storage.NewMemoryStorage(), realtime.NewHub())
	req := httptest.NewRequest(http.MethodPost, "/pdf", strings.NewReader(`{"url":"https://example.com"}`))
	rr := httptest.NewRecorder()

	h.SubmitJob(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestHealthz(t *testing.T) {
	h := NewHandler(queue.NewMemoryQueue(1), storage.NewMemoryStorage(), realtime.NewHub())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	h.Healthz(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
