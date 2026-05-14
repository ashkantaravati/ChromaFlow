package blob

import (
	"context"
	"errors"
	"sync"
)

type MemoryStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{data: make(map[string][]byte)} }

func (s *MemoryStore) Put(ctx context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.data[key]
	if !ok {
		return nil, errors.New("object not found")
	}
	return append([]byte(nil), data...), nil
}

func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}
