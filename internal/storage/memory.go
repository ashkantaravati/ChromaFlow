package storage

import (
	"chromaflow/internal/observability"
	"chromaflow/internal/queue"
	"errors"
	"sort"
	"sync"
	"time"
)

type MemoryStorage struct {
	mu      sync.RWMutex
	results map[string]*queue.JobResult
	order   []string
	onSet   func()
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		results: make(map[string]*queue.JobResult),
	}
}

func (s *MemoryStorage) SetOnChange(onSet func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSet = onSet
}

func (s *MemoryStorage) Set(jobID string, result *queue.JobResult) error {
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

	s.results[jobID] = result
	onSet := s.onSet
	s.mu.Unlock()

	if onSet != nil {
		onSet()
	}
	return nil
}

func (s *MemoryStorage) Delete(jobID string) {
	s.mu.Lock()
	if _, ok := s.results[jobID]; !ok {
		s.mu.Unlock()
		return
	}
	delete(s.results, jobID)
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
}

func (s *MemoryStorage) Get(jobID string) (*queue.JobResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result, ok := s.results[jobID]
	if !ok {
		return nil, errors.New("job not found")
	}
	return result, nil
}

func (s *MemoryStorage) Stats() observability.StorageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := observability.StorageStats{
		Total:    len(s.results),
		ByStatus: make(map[queue.JobStatus]int),
	}
	for _, result := range s.results {
		stats.ByStatus[result.Status]++
		stats.PDFBytes += len(result.PDF)
		if stats.OldestJobAt.IsZero() || result.CreatedAt.Before(stats.OldestJobAt) {
			stats.OldestJobAt = result.CreatedAt
		}
	}

	return stats
}

func (s *MemoryStorage) List() []queue.JobSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]queue.JobSnapshot, 0, len(s.results))
	for _, jobID := range s.order {
		result, ok := s.results[jobID]
		if !ok {
			continue
		}
		jobs = append(jobs, queue.JobSnapshot{
			ID:        jobID,
			URL:       result.URL,
			Status:    result.Status,
			Error:     result.Error,
			CreatedAt: result.CreatedAt,
			UpdatedAt: result.UpdatedAt,
		})
	}

	sort.SliceStable(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})

	return jobs
}
