package handler

import (
	"context"
	"sync"
	"testing"

	"github.com/Aryan9inja/gotaskq/internal/job"
)

type recordingRegistrar struct {
	count int
}

type MockHandler struct {
	handleFunc func(ctx context.Context, job *job.Job) error
}

func (h *MockHandler) Handle(ctx context.Context, job *job.Job) error {
	return h.handleFunc(ctx, job)
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()

	t.Run("Concurrency", func (t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 100

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func (i int)  {
				defer wg.Done()
				jobType := "type" + string(rune(i))
				reg.Register(jobType, &MockHandler{})
				reg.Get(jobType)
			}(i)
		}

		wg.Wait()
	})
}