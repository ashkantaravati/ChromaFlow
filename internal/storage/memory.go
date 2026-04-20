package storage

import (
	"chromaflow/internal/queue"
	"errors"
	"sync"
)

type MemoryStorage struct {
	mu      sync.RWMutex
	results map[string]*queue.JobResult
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		results: make(map[string]*queue.JobResult),
	}
}

func (s *MemoryStorage) Set(jobID string, result *queue.JobResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[jobID] = result
	return nil
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
