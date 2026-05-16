package dlq_test

import (
	"context"
	"testing"

	"github.com/Aryan9inja/gotaskq/internal/dlq"
	"github.com/Aryan9inja/gotaskq/internal/job"
	testutils "github.com/Aryan9inja/gotaskq/internal/testUtils"
)

func TestRedisDlq(t *testing.T) {
	client := testutils.GetRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	t.Run("Save and Get Job", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		d, err := dlq.NewRedisDlq(client)
		if err != nil {
			t.Fatalf("Failed to create RedisDlq: %v", err)
		}

		j1 := testutils.NewTestJob("j1", 5, 0)
		j1.Status = job.StatusDead
		if err := d.Save(ctx, j1); err != nil {
			t.Fatalf("Failed to save job: %v", err)
		}

		got, err := d.Get(ctx, j1.ID)
		if err != nil {
			t.Fatalf("Failed to get job: %v", err)
		}
		if got.ID != j1.ID {
			t.Errorf("Expected job ID %s, got %s", j1.ID, got.ID)
		}
	})

	t.Run("Save invalid status job", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		d, err := dlq.NewRedisDlq(client)
		if err != nil {
			t.Fatalf("Failed to create RedisDlq: %v", err)
		}

		j1 := testutils.NewTestJob("j1", 5, 0)
		j1.Status = job.StatusPending
		err = d.Save(ctx, j1)
		if err == nil {
			t.Fatal("Expected error when saving job with non-dead status, got nil")
		}
	})

	t.Run("Delete Job", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		d, err := dlq.NewRedisDlq(client)
		if err != nil {
			t.Fatalf("Failed to create RedisDlq: %v", err)
		}
		
		j1 := testutils.NewTestJob("j1", 5, 0)
		j1.Status = job.StatusDead
		if err := d.Save(ctx, j1); err != nil {
			t.Fatalf("Failed to save job: %v", err)
		}

		if err := d.Delete(ctx, j1.ID); err != nil {
			t.Fatalf("Failed to delete job: %v", err)
		}

		_, err = d.Get(ctx, j1.ID)
		if err == nil {
			t.Fatal("Expected error when getting deleted job, got nil")
		}
	})

	t.Run("List Jobs", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		d, err := dlq.NewRedisDlq(client)
		if err != nil {
			t.Fatalf("Failed to create RedisDlq: %v", err)
		}

		j1 := testutils.NewTestJob("j1", 5, 0)
		j1.Status = job.StatusDead
		j2 := testutils.NewTestJob("j2", 5, 0)
		j2.Status = job.StatusDead

		if err := d.Save(ctx, j1); err != nil {
			t.Fatalf("Failed to save job j1: %v", err)
		}
		if err := d.Save(ctx, j2); err != nil {
			t.Fatalf("Failed to save job j2: %v", err)
		}

		jobs, err := d.List(ctx, 10)
		if err != nil {
			t.Fatalf("Failed to list jobs: %v", err)
		}
		if len(jobs) != 2 {
			t.Errorf("Expected 2 jobs in list, got %d", len(jobs))
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		d, err := dlq.NewRedisDlq(client)
		if err != nil {
			t.Fatalf("Failed to create RedisDlq: %v", err)
		}

		_, err = d.Get(ctx, "nonexistent")
		if err == nil {
			t.Fatal("Expected error when getting non-existent job, got nil")
		}
	})
}
