package api

import (
	"chromaflow/internal/blob"
	"chromaflow/internal/observability"
	"chromaflow/internal/queue"
	"chromaflow/internal/realtime"
	"chromaflow/internal/storage"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Handler struct {
	queue   queue.Backend
	storage storage.Store
	blobs   blob.Store
	hub     *realtime.Hub
	metrics *observability.Metrics
	logger  *slog.Logger
}

func NewHandler(q queue.Backend, s storage.Store, blobs blob.Store, hub *realtime.Hub, metrics *observability.Metrics, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{queue: q, storage: s, blobs: blobs, hub: hub, metrics: metrics, logger: logger}
}

type SubmitRequest struct {
	URL            string `json:"url"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type SubmitResponse struct {
	JobID      string `json:"job_id"`
	StatusURL  string `json:"status_url"`
	Idempotent bool   `json:"idempotent,omitempty"`
}

func (h *Handler) SubmitJob(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.recordRejected("invalid_json")
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	req.URL = strings.TrimSpace(req.URL)
	if err := validateURL(req.URL); err != nil {
		h.recordRejected("invalid_url")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}

	jobID := uuid.New().String()
	reservedJobID, reserved, err := h.storage.ReserveIdempotencyKey(r.Context(), idempotencyKey, jobID)
	if err != nil {
		h.recordRejected("idempotency_error")
		h.logger.Error("idempotency reservation failed", slog.String("error", err.Error()))
		http.Error(w, "Failed to reserve idempotency key", http.StatusInternalServerError)
		return
	}
	if !reserved {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(SubmitResponse{JobID: reservedJobID, StatusURL: "/pdf/" + reservedJobID, Idempotent: true})
		return
	}

	job := queue.Job{ID: jobID, URL: req.URL, IdempotencyKey: idempotencyKey}
	now := time.Now().UTC()

	// Initialize job as pending
	_ = h.storage.Set(r.Context(), jobID, &queue.JobResult{
		URL:            req.URL,
		Status:         queue.StatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
		IdempotencyKey: idempotencyKey,
	})

	// Push to queue
	if err := h.queue.Push(r.Context(), job); err != nil {
		_ = h.storage.Delete(r.Context(), jobID)
		if errors.Is(err, queue.ErrQueueFull) {
			h.recordRejected("queue_full")
			h.logger.Warn("job rejected", slog.String("reason", "queue_full"), slog.String("job_id", jobID))
			http.Error(w, "Queue is full", http.StatusServiceUnavailable)
			return
		}
		h.recordRejected("enqueue_error")
		h.logger.Error("job enqueue failed", slog.String("job_id", jobID), slog.String("error", err.Error()))
		http.Error(w, "Failed to queue job", http.StatusInternalServerError)
		return
	}

	h.recordSubmitted()
	h.logger.Info("job queued", slog.String("job_id", jobID), slog.String("url", req.URL))

	resp := SubmitResponse{
		JobID:     jobID,
		StatusURL: "/pdf/" + jobID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")

	result, err := h.storage.Get(r.Context(), jobID)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	if result.Status == queue.StatusCompleted {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", "attachment; filename="+jobID+".pdf")
		pdfBytes, err := h.blobs.Get(r.Context(), result.PDFKey)
		if err != nil {
			h.logger.Error("pdf fetch failed", slog.String("job_id", jobID), slog.String("error", err.Error()))
			http.Error(w, "PDF not found", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(pdfBytes)
		return
	}

	// Return status as JSON
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"job_id": jobID,
		"url":    result.URL,
		"status": result.Status,
		"error":  result.Error,
	})
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dashboardTemplate.Execute(w, nil); err != nil {
		http.Error(w, "Failed to render dashboard", http.StatusInternalServerError)
	}
}

func (h *Handler) JobsWebSocket(w http.ResponseWriter, r *http.Request) {
	h.hub.ServeJobs(w, r, func() []queue.JobSnapshot { return h.storage.List(r.Context()) })
}

func (h *Handler) CancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	result, err := h.storage.Cancel(r.Context(), jobID)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"job_id": jobID,
		"url":    result.URL,
		"status": result.Status,
		"error":  result.Error,
	})
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func (h *Handler) OpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPISpec)
}

func (h *Handler) recordSubmitted() {
	if h.metrics != nil {
		h.metrics.JobSubmitted()
	}
}

func (h *Handler) recordRejected(reason string) {
	if h.metrics != nil {
		h.metrics.JobRejected(reason)
	}
}

func validateURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("URL is required")
	}

	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return errors.New("URL must be absolute and valid")
	}
	if parsed.Host == "" {
		return errors.New("URL must include a host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("URL scheme must be http or https")
	}

	return nil
}

var dashboardTemplate = template.Must(template.New("dashboard").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Chromaflow</title>
  <style>
    :root { color-scheme: light dark; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: #101827; color: #e5edf8; }
    main { width: min(1040px, calc(100% - 32px)); margin: 0 auto; padding: 48px 0; }
    h1 { margin: 0 0 8px; font-size: clamp(2rem, 6vw, 4rem); letter-spacing: -0.06em; }
    p { color: #9fb0c6; }
    .panel { background: rgba(18, 29, 47, 0.92); border: 1px solid #26364f; border-radius: 20px; box-shadow: 0 18px 60px rgba(0,0,0,0.24); padding: 24px; margin-top: 24px; }
    form { display: grid; grid-template-columns: 1fr auto; gap: 12px; }
    input { border: 1px solid #334762; background: #0b1220; color: #eef6ff; border-radius: 12px; padding: 14px 16px; font: inherit; outline: none; }
    input:focus { border-color: #65a8ff; box-shadow: 0 0 0 4px rgba(101,168,255,0.14); }
    button { border: 0; border-radius: 12px; padding: 14px 18px; font: inherit; font-weight: 700; color: #06101f; background: linear-gradient(135deg, #73d6ff, #8bffbd); cursor: pointer; }
    button:disabled { cursor: wait; opacity: .65; }
    .status-bar { display: flex; justify-content: space-between; gap: 16px; align-items: center; margin: 18px 0 4px; color: #9fb0c6; font-size: .95rem; }
    .jobs { display: grid; gap: 12px; margin-top: 16px; }
    .job { border: 1px solid #26364f; background: #0b1220; border-radius: 16px; padding: 16px; display: grid; gap: 10px; }
    .job-header { display: flex; justify-content: space-between; gap: 12px; align-items: start; }
    .url { overflow-wrap: anywhere; color: #d8e7fb; }
    .meta { color: #7f91aa; font-size: .85rem; }
    .badge { border-radius: 999px; padding: 5px 10px; font-size: .78rem; font-weight: 800; text-transform: uppercase; letter-spacing: .04em; white-space: nowrap; }
    .pending { background: #3a2d12; color: #ffd98c; }
    .processing { background: #102e4f; color: #82c7ff; }
    .completed { background: #113721; color: #94f4ad; }
    .failed { background: #41181f; color: #ff9dac; }
    .error { color: #ff9dac; font-size: .9rem; overflow-wrap: anywhere; }
    a { color: #8fd5ff; }
    @media (max-width: 680px) { form { grid-template-columns: 1fr; } .job-header { flex-direction: column; } }
  </style>
</head>
<body>
  <main>
    <h1>Chromaflow</h1>
    <p>Submit a URL, watch the queue update in real time, and download the generated PDF when it completes.</p>

    <section class="panel">
      <form id="job-form">
        <input id="url" name="url" type="url" placeholder="https://example.com" required>
        <button id="submit" type="submit">Create PDF</button>
      </form>
      <div class="status-bar">
        <span id="notice">Ready</span>
        <span id="socket-status">Connecting…</span>
      </div>
    </section>

    <section class="panel">
      <h2>Jobs</h2>
      <div id="jobs" class="jobs"><p>No jobs yet.</p></div>
    </section>
  </main>

  <script>
    const form = document.querySelector('#job-form');
    const urlInput = document.querySelector('#url');
    const submitButton = document.querySelector('#submit');
    const notice = document.querySelector('#notice');
    const socketStatus = document.querySelector('#socket-status');
    const jobsContainer = document.querySelector('#jobs');

    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      submitButton.disabled = true;
      notice.textContent = 'Submitting job…';

      try {
        const response = await fetch('/pdf', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ url: urlInput.value })
        });

        if (!response.ok) {
          throw new Error(await response.text());
        }

        const job = await response.json();
        notice.textContent = 'Queued ' + job.job_id;
        urlInput.value = '';
      } catch (error) {
        notice.textContent = 'Submit failed: ' + error.message;
      } finally {
        submitButton.disabled = false;
        urlInput.focus();
      }
    });

    function connect() {
      const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
      const socket = new WebSocket(scheme + '://' + location.host + '/ws/jobs');

      socket.addEventListener('open', () => {
        socketStatus.textContent = 'Live';
      });

      socket.addEventListener('message', (event) => {
        const message = JSON.parse(event.data);
        if (message.type === 'jobs') {
          renderJobs(message.jobs || []);
        }
      });

      socket.addEventListener('close', () => {
        socketStatus.textContent = 'Reconnecting…';
        setTimeout(connect, 1000);
      });

      socket.addEventListener('error', () => {
        socket.close();
      });
    }

    function renderJobs(jobs) {
      if (jobs.length === 0) {
        jobsContainer.innerHTML = '<p>No jobs yet.</p>';
        return;
      }

      jobsContainer.replaceChildren(...jobs.map(renderJob));
    }

    function renderJob(job) {
      const article = document.createElement('article');
      article.className = 'job';

      const header = document.createElement('div');
      header.className = 'job-header';

      const left = document.createElement('div');
      const url = document.createElement('div');
      url.className = 'url';
      url.textContent = job.url || '(unknown URL)';
      const meta = document.createElement('div');
      meta.className = 'meta';
      meta.textContent = job.id + ' · updated ' + formatTime(job.updated_at);
      left.append(url, meta);

      const badge = document.createElement('span');
      badge.className = 'badge ' + job.status;
      badge.textContent = job.status;
      header.append(left, badge);
      article.append(header);

      if (job.status === 'completed') {
        const link = document.createElement('a');
        link.href = '/pdf/' + job.id;
        link.textContent = 'Download PDF';
        article.append(link);
      }

      if (job.error) {
        const error = document.createElement('div');
        error.className = 'error';
        error.textContent = job.error;
        article.append(error);
      }

      return article;
    }

    function formatTime(value) {
      if (!value) return 'just now';
      return new Intl.DateTimeFormat(undefined, { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value));
    }

    connect();
  </script>
</body>
</html>`))
