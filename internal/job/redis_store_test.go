package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/job"
	testutils "github.com/Aryan9inja/gotaskq/internal/testUtils"
)

func TestRedisStore(t *testing.T){
	client := testutils.GetRedisClient(t)
	defer client.Close()

	ctx := context.Background()
	ttl := 1*time.Hour

	t.Run("Save and Get", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		store, _ := job.NewRedisStore(client, ttl)

		j1 := testutils.NewTestJob("j1", 5, 0)
		if err := store.Save(ctx, j1); err!=nil{
			t.Fatalf("Save failed: %v", err)
		}

		got, err := store.Get(ctx, j1.ID)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if got.ID != j1.ID || got.Priority!=j1.Priority || got.Type != j1.Type{
			t.Errorf("Job Mismatch, expected %+v, got %+v",j1, got)
		}
	})

	t.Run("Update Status", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		store, _ := job.NewRedisStore(client, ttl)

		j1 := testutils.NewTestJob("j1", 5, 0)
		store.Save(ctx,j1)

		// Valid Transition
		if err := store.UpdateStatus(ctx, "j1", job.StatusRunning); err!=nil{
			t.Errorf("Expected valid transition to succeed, got error: %v", err)
		}

		got, _ := store.Get(ctx, "j1")
		if got.Status != job.StatusRunning{
			t.Errorf("Expected status to be RUNNING, got %s", got.Status)
		}

		err := store.UpdateStatus(ctx, "j2", job.StatusPending)
		if err == nil {
			t.Error("Expected error for invalid transition")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		store, _ := job.NewRedisStore(client, 1*time.Second)

		j1 := testutils.NewTestJob("j1", 5, 0)
		store.Save(ctx,j1)

		if err := store.Delete(ctx, "j1"); err!=nil{
			t.Fatalf("Delete failed: %v", err)
		}

		time.Sleep(1005 * time.Millisecond)

		_, err := store.Get(ctx, "j1")
		if err == nil {
			t.Error("Expected job to be deleted/expired, but it was found")
		}
	})

	t.Run("Not found", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		store, _ := job.NewRedisStore(client, ttl)

		_, err := store.Get(ctx, "nonexistent")
		if err == nil {
			t.Errorf("Expected error for non-existent job: %v", err)
		}
	})
}