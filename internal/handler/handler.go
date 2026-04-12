package handler

import (
	"context"
	"sync"

	"github.com/Aryan9inja/gotaskq/internal/job"
)

type Handler interface {
	Handle(ctx context.Context, job *job.Job) error
}

type Registry struct {
	mu      sync.RWMutex
	handler map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{
		handler: make(map[string]Handler),
	}
}

func (reg *Registry) Register(jobType string, hand Handler) {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	reg.handler[jobType] = hand
}

func (reg *Registry) Get(jobType string) (Handler, bool) {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	h, ok := reg.handler[jobType]
	return h, ok
}
