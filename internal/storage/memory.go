package storage

import (
	"chromaflow/internal/observability"
	"chromaflow/internal/queue"
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

type MemoryStorage struct {
	mu          sync.RWMutex
	results     map[string]*queue.JobResult
	idempotency map[string]string
	order       []string
	onSet       func()
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{results: make(map[string]*queue.JobResult), idempotency: make(map[string]string)}
}

func (s *MemoryStorage) SetOnChange(onSet func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSet = onSet
}

func (s *MemoryStorage) Set(ctx context.Context, jobID string, result *queue.JobResult) error {
	s.mu.Lock()
	_, exists := s.results[jobID]
	if !exists {
		s.order = append(s.order, jobID)
	}

	now := time.Now().UTC()
	if result.CreatedAt.IsZero() {
		if existing, ok := s.results[jobID]; ok && !existing.CreatedAt.IsZero() {
			result.CreatedAt = existing.CreatedAt
		} else {
			result.CreatedAt = now
		}
	}
	result.UpdatedAt = now
	if result.IdempotencyKey == "" {
		if existing, ok := s.results[jobID]; ok {
			result.IdempotencyKey = existing.IdempotencyKey
		}
	}

	s.results[jobID] = result
	onSet := s.onSet
	s.mu.Unlock()

	if onSet != nil {
		onSet()
	}
	return nil
}

func (s *MemoryStorage) Delete(ctx context.Context, jobID string) error {
	s.mu.Lock()
	result, ok := s.results[jobID]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.results, jobID)
	if result.IdempotencyKey != "" {
		delete(s.idempotency, result.IdempotencyKey)
	}
	for i, id := range s.order {
		if id == jobID {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	onSet := s.onSet
	s.mu.Unlock()

	if onSet != nil {
		onSet()
	}
	return nil
}

func (s *MemoryStorage) Get(ctx context.Context, jobID string) (*queue.JobResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result, ok := s.results[jobID]
	if !ok {
		return nil, errors.New("job not found")
	}
	copy := *result
	return &copy, nil
}

func (s *MemoryStorage) Cancel(ctx context.Context, jobID string) (*queue.JobResult, error) {
	result, err := s.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if result.Status == queue.StatusCompleted || result.Status == queue.StatusFailed || result.Status == queue.StatusCanceled {
		return result, nil
	}
	result.Status = queue.StatusCanceled
	result.Error = "job canceled"
	if err := s.Set(ctx, jobID, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *MemoryStorage) ReserveIdempotencyKey(ctx context.Context, key, jobID string) (string, bool, error) {
	if key == "" {
		return "", true, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.idempotency[key]; ok {
		return existing, false, nil
	}
	s.idempotency[key] = jobID
	return jobID, true, nil
}

func (s *MemoryStorage) Stats(ctx context.Context) observability.StorageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := observability.StorageStats{Total: len(s.results), ByStatus: make(map[queue.JobStatus]int)}
	for _, result := range s.results {
		stats.ByStatus[result.Status]++
		stats.PDFBytes += result.PDFSize + len(result.PDF)
		if stats.OldestJobAt.IsZero() || result.CreatedAt.Before(stats.OldestJobAt) {
			stats.OldestJobAt = result.CreatedAt
		}
	}
	return stats
}

func (s *MemoryStorage) List(ctx context.Context) []queue.JobSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]queue.JobSnapshot, 0, len(s.results))
	for _, jobID := range s.order {
		result, ok := s.results[jobID]
		if !ok {
			continue
		}
		jobs = append(jobs, queue.JobSnapshot{ID: jobID, URL: result.URL, Status: result.Status, Error: result.Error, CreatedAt: result.CreatedAt, UpdatedAt: result.UpdatedAt})
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].CreatedAt.After(jobs[j].CreatedAt) })
	return jobs
}
