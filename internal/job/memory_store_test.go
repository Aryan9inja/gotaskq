package job

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()

	t.Run("Save and Get", func(t *testing.T) {
		store := NewMemoryStore()
		j := &Job{ID: "j1", Type: "test", Status: StatusPending}

		if err := store.Save(ctx, j); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		got, err := store.Get(ctx, j.ID)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if got.ID != j.ID {
			t.Fatalf("Expected ID: %s, got %s", j.ID, got.ID)
		}
	})

	t.Run("Update Status Valid", func(t *testing.T) {
		store := NewMemoryStore()
		j := &Job{ID: "j2", Type: "test", Status: StatusPending}
		store.Save(ctx, j)

		if err := store.UpdateStatus(ctx, "j2", StatusRunning); err != nil {
			t.Fatalf("Update status failed: %v", err)
		}

		got, _ := store.Get(ctx, j.ID)
		if got.Status != StatusRunning {
			t.Fatalf("Expected status %s, got %s", StatusRunning, got.Status)
		}
	})

	t.Run("Update Status Invalid", func(t *testing.T) {
		store := NewMemoryStore()
		j := &Job{ID: "j2", Type: "test", Status: StatusPending}
		store.Save(ctx, j)

		err := store.UpdateStatus(ctx, "j2", StatusDone)
		if err == nil {
			t.Error("Expected error for invalid transition")
		}
	})
}

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
