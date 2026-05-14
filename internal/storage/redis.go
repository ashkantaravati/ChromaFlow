package storage

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"chromaflow/internal/observability"
	"chromaflow/internal/queue"
	"chromaflow/internal/redisx"
)

type RedisStorage struct {
	client *redisx.Client
	prefix string
	onSet  func()
}

func NewRedisStorage(rawURL, prefix string) (*RedisStorage, error) {
	client, err := redisx.New(rawURL)
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		prefix = "chromaflow"
	}
	return &RedisStorage{client: client, prefix: prefix}, nil
}

func (s *RedisStorage) SetOnChange(onSet func()) { s.onSet = onSet }

func (s *RedisStorage) Set(ctx context.Context, jobID string, result *queue.JobResult) error {
	now := time.Now().UTC()
	existing, _ := s.Get(ctx, jobID)
	createdAt := result.CreatedAt
	if createdAt.IsZero() {
		if existing != nil && !existing.CreatedAt.IsZero() {
			createdAt = existing.CreatedAt
		} else {
			createdAt = now
		}
	}
	idempotencyKey := result.IdempotencyKey
	callbackURL := result.CallbackURL
	requestID := result.RequestID
	if existing != nil {
		if idempotencyKey == "" {
			idempotencyKey = existing.IdempotencyKey
		}
		if callbackURL == "" {
			callbackURL = existing.CallbackURL
		}
		if requestID == "" {
			requestID = existing.RequestID
		}
	}
	if existing == nil {
		_, _ = s.client.Do(ctx, "LPUSH", s.orderKey(), jobID)
	}
	_, err := s.client.Do(ctx, "HSET", s.jobKey(jobID),
		"url", result.URL,
		"status", string(result.Status),
		"pdf_key", result.PDFKey,
		"pdf_size", strconv.Itoa(result.PDFSize),
		"error", result.Error,
		"idempotency_key", idempotencyKey,
		"callback_url", callbackURL,
		"request_id", requestID,
		"created_at", createdAt.Format(time.RFC3339Nano),
		"updated_at", now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	if s.onSet != nil {
		s.onSet()
	}
	return nil
}

func (s *RedisStorage) Delete(ctx context.Context, jobID string) error {
	result, _ := s.Get(ctx, jobID)
	_, err := s.client.Do(ctx, "DEL", s.jobKey(jobID))
	_, _ = s.client.Do(ctx, "LREM", s.orderKey(), "0", jobID)
	if result != nil && result.IdempotencyKey != "" {
		_, _ = s.client.Do(ctx, "DEL", s.idempotencyKey(result.IdempotencyKey))
	}
	if s.onSet != nil {
		s.onSet()
	}
	return err
}

func (s *RedisStorage) Get(ctx context.Context, jobID string) (*queue.JobResult, error) {
	v, err := s.client.Do(ctx, "HGETALL", s.jobKey(jobID))
	if err != nil {
		return nil, err
	}
	arr, _ := v.([]any)
	if len(arr) == 0 {
		return nil, fmt.Errorf("job not found")
	}
	m := make(map[string]string, len(arr)/2)
	for i := 0; i+1 < len(arr); i += 2 {
		m[redisx.String(arr[i])] = redisx.String(arr[i+1])
	}
	createdAt, _ := time.Parse(time.RFC3339Nano, m["created_at"])
	updatedAt, _ := time.Parse(time.RFC3339Nano, m["updated_at"])
	pdfSize, _ := strconv.Atoi(m["pdf_size"])
	return &queue.JobResult{URL: m["url"], Status: queue.JobStatus(m["status"]), PDFKey: m["pdf_key"], PDFSize: pdfSize, Error: m["error"], IdempotencyKey: m["idempotency_key"], CallbackURL: m["callback_url"], RequestID: m["request_id"], CreatedAt: createdAt, UpdatedAt: updatedAt}, nil
}

func (s *RedisStorage) Cancel(ctx context.Context, jobID string) (*queue.JobResult, error) {
	result, err := s.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if result.Status == queue.StatusCompleted || result.Status == queue.StatusFailed || result.Status == queue.StatusCanceled {
		return result, nil
	}
	result.Status = queue.StatusCanceled
	result.Error = "job canceled"
	return result, s.Set(ctx, jobID, result)
}

func (s *RedisStorage) ReserveIdempotencyKey(ctx context.Context, key, jobID string) (string, bool, error) {
	if key == "" {
		return "", true, nil
	}
	v, err := s.client.Do(ctx, "SET", s.idempotencyKey(key), jobID, "NX")
	if err != nil {
		return "", false, err
	}
	if redisx.String(v) == "OK" {
		return jobID, true, nil
	}
	existing, err := s.client.Do(ctx, "GET", s.idempotencyKey(key))
	if err != nil {
		return "", false, err
	}
	return redisx.String(existing), false, nil
}

func (s *RedisStorage) Stats(ctx context.Context) observability.StorageStats {
	jobs := s.List(ctx)
	stats := observability.StorageStats{Total: len(jobs), ByStatus: make(map[queue.JobStatus]int)}
	for _, job := range jobs {
		stats.ByStatus[job.Status]++
		if stats.OldestJobAt.IsZero() || job.CreatedAt.Before(stats.OldestJobAt) {
			stats.OldestJobAt = job.CreatedAt
		}
		if result, err := s.Get(ctx, job.ID); err == nil {
			stats.PDFBytes += result.PDFSize
		}
	}
	return stats
}

func (s *RedisStorage) List(ctx context.Context) []queue.JobSnapshot {
	v, err := s.client.Do(ctx, "LRANGE", s.orderKey(), "0", "199")
	if err != nil {
		return nil
	}
	ids, _ := v.([]any)
	jobs := make([]queue.JobSnapshot, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, idv := range ids {
		id := redisx.String(idv)
		if seen[id] {
			continue
		}
		seen[id] = true
		result, err := s.Get(ctx, id)
		if err != nil {
			continue
		}
		jobs = append(jobs, queue.JobSnapshot{ID: id, URL: result.URL, Status: result.Status, Error: result.Error, CreatedAt: result.CreatedAt, UpdatedAt: result.UpdatedAt})
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].CreatedAt.After(jobs[j].CreatedAt) })
	return jobs
}

func (s *RedisStorage) jobKey(jobID string) string       { return s.prefix + ":job:" + jobID }
func (s *RedisStorage) orderKey() string                 { return s.prefix + ":jobs" }
func (s *RedisStorage) idempotencyKey(key string) string { return s.prefix + ":idempotency:" + key }
