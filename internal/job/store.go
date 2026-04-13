package job

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type Store interface {
	Save(ctx context.Context, job *Job) error
	Get(ctx context.Context, id string) (*Job, error)
	UpdateStatus(ctx context.Context, id string, status Status) error
	Delete(ctx context.Context, id string) error
}

type inMemoryStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func (store *inMemoryStore) Save(ctx context.Context, job *Job) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.jobs[job.ID] = job

	return nil
}

func (store *inMemoryStore) Get(ctx context.Context, id string) (*Job, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	job, exists := store.jobs[id]
	if !exists {
		return nil, errors.New("job not found")
	}

	return job, nil
}

func isValidTransaction(from, to Status) bool {
	switch from {
	case StatusPending:
		return to == StatusRunning
	case StatusRunning:
		return to == StatusDone || to == StatusFailed
	case StatusFailed:
		return to == StatusPending || to == StatusDead
	default:
		return false
	}
}

func (store *inMemoryStore) UpdateStatus(ctx context.Context, id string, status Status) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	job, err := store.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("job with id = %s does not exist", id)
	}

	if !isValidTransaction(job.Status, status) {
		return fmt.Errorf("invalid transaction: %s -> %s", string(job.Status), string(status))
	}

	job.Status = status

	return nil
}

func(store *inMemoryStore) Delete(ctx context.Context, id string) error{
	store.mu.Lock()
	defer store.mu.Unlock()

	delete(store.jobs, id)

	return nil
}
