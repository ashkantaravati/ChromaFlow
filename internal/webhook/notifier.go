package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"chromaflow/internal/queue"
)

type Notifier struct {
	client *http.Client
}

type Event struct {
	JobID     string          `json:"job_id"`
	URL       string          `json:"url"`
	Status    queue.JobStatus `json:"status"`
	Error     string          `json:"error,omitempty"`
	PDFKey    string          `json:"pdf_key,omitempty"`
	PDFSize   int             `json:"pdf_size,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
}

func NewNotifier(timeout time.Duration) *Notifier {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Notifier{client: &http.Client{Timeout: timeout}}
}

func (n *Notifier) Notify(ctx context.Context, callbackURL string, event Event) error {
	if n == nil || callbackURL == "" {
		return nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if event.RequestID != "" {
		req.Header.Set("X-Request-ID", event.RequestID)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
