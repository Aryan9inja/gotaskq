package job

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryStoreDelete(t *testing.T) {
	t.Run("retains job until ttl", func(t *testing.T) {
		ctx := context.Background()
		store := NewMemoryStore(25 * time.Millisecond)
		j := &Job{
			ID:     "job-1",
			Type:   "logger",
			Status: StatusDone,
		}

		if err := store.Save(ctx, j); err != nil {
			t.Fatalf("save job: %v", err)
		}
		if err := store.Delete(ctx, j.ID); err != nil {
			t.Fatalf("delete job: %v", err)
		}

		if _, err := store.Get(ctx, j.ID); err != nil {
			t.Fatalf("expected job to be retained before ttl: %v", err)
		}

		time.Sleep(75 * time.Millisecond)

		if _, err := store.Get(ctx, j.ID); !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("expected job to be removed after ttl: %v", err)
		}
	})

	t.Run("remove job immediatly without ttl", func(t *testing.T) {
		ctx := context.Background()
		store := NewMemoryStore()
		j := &Job{
			ID:     "job-1",
			Type:   "logger",
			Status: StatusDone,
		}

		if err := store.Save(ctx, j); err != nil {
			t.Fatalf("save job: %v", err)
		}
		if err := store.Delete(ctx, j.ID); err != nil {
			t.Fatalf("delete job: %v", err)
		}

		if _, err := store.Get(ctx, j.ID); !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("expected job to be removed immediatly: %v", err)
		}
	})
}
