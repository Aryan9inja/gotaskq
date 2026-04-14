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

func validateContext(ctx context.Context) error{
	if ctx == nil{
		return errors.New("Context is null")
	}

	select{
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return nil
}

func (store *inMemoryStore) Save(ctx context.Context, job *Job) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	err:=validateContext(ctx)
	if err != nil {
		return err
	}

	store.jobs[job.ID] = job

	return nil
}

func (store *inMemoryStore) Get(ctx context.Context, id string) (*Job, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	err:=validateContext(ctx)
	if err != nil {
		return nil, err
	}

	job, exists := store.jobs[id]
	if !exists {
		return nil, errors.New("job not found")
	}

	return job, nil
}

func isValidTransition(from, to Status) bool {
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

	err:=validateContext(ctx)
	if err != nil {
		return err
	}

	job, exists := store.jobs[id]
	if !exists {
		return fmt.Errorf("job with id = %s does not exist", id)
	}

	if !isValidTransition(job.Status, status) {
		return fmt.Errorf("invalid transition: %s -> %s", string(job.Status), string(status))
	}

	job.Status = status

	return nil
}

func(store *inMemoryStore) Delete(ctx context.Context, id string) error{
	store.mu.Lock()
	defer store.mu.Unlock()

	err:=validateContext(ctx)
	if err != nil {
		return err
	}

	delete(store.jobs, id)

	return nil
}
