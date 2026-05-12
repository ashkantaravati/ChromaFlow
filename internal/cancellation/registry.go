package cancellation

import (
	"context"
	"sync"
)

type Registry struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewRegistry() *Registry {
	return &Registry{cancels: make(map[string]context.CancelFunc)}
}

func (r *Registry) Register(jobID string, cancel context.CancelFunc) func() {
	r.mu.Lock()
	r.cancels[jobID] = cancel
	r.mu.Unlock()
	return func() { r.Unregister(jobID) }
}

func (r *Registry) Unregister(jobID string) {
	r.mu.Lock()
	delete(r.cancels, jobID)
	r.mu.Unlock()
}

func (r *Registry) Cancel(jobID string) bool {
	r.mu.Lock()
	cancel, ok := r.cancels[jobID]
	r.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}
