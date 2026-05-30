package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/job"
	testutils "github.com/Aryan9inja/gotaskq/internal/testUtils"
)

func TestStatusUpdate(t *testing.T) {
	client := testutils.GetRedisClient(t)
	defer client.Close()

	ctx := context.Background()
	ttl := 1 * time.Hour

	t.Run("State Transitions", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		st, err := job.NewRedisStore(client, ttl)
		if err != nil {
			t.Fatalf("NewRedisStore failed: %v", err)
		}

		// Initial job status is "pending"
		j1 := testutils.NewTestJob("j1", 5, 0)
		if err := st.Save(ctx, j1); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// 1. Pending -> Running (Valid	Transition)
		if err := st.UpdateStatus(ctx, j1.ID, job.StatusRunning); err != nil {
			t.Errorf("PENDING -> RUNNING should be valid, got: %v", err)
		}

		// 2. Running -> Pending (Invalid Transition)
		if err := st.UpdateStatus(ctx, j1.ID, job.StatusPending); err == nil {
			t.Errorf("RUNNING -> PENDING should be invalid, but no error was returned")
		}

		// 3. Running -> Done (Valid Transition)
		if err := st.UpdateStatus(ctx, j1.ID, job.StatusDone); err != nil {
			t.Errorf("RUNNING -> DONE should be valid, got: %v", err)
		}

		// 4. Done -> Running (Invalid Transition)
		if err := st.UpdateStatus(ctx, j1.ID, job.StatusRunning); err == nil {
			t.Errorf("DONE -> RUNNING should be invalid, but no error was returned")
		}

		// 5. Done -> Pending (Invalid Transition)
		if err := st.UpdateStatus(ctx, j1.ID, job.StatusPending); err == nil {
			t.Errorf("DONE -> PENDING should be invalid, but no error was returned")
		}

		// Setting status back to "failed" for next test
		j1.Status = job.StatusFailed
		if err := st.Save(ctx, j1); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		// 6. Failed -> Pending (Valid Transition)
		if err := st.UpdateStatus(ctx, j1.ID, job.StatusPending); err != nil {
			t.Errorf("FAILED -> PENDING should be valid, got: %v", err)
		}

		// Setting status back to "failed" for next tests
		j1.Status = job.StatusFailed
		if err := st.Save(ctx, j1); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		// 7. Failed -> Running (Invalid Transition)
		if err := st.UpdateStatus(ctx, j1.ID, job.StatusRunning); err == nil {
			t.Errorf("FAILED -> RUNNING should be invalid, but no error was returned")
		}

		// 8. Failed -> Done (Invalid Transition)
		if err := st.UpdateStatus(ctx, j1.ID, job.StatusDone); err == nil {
			t.Errorf("FAILED -> DONE should be invalid, but no error was returned")
		}

		// 9. Failed -> Dead (Valid Transition)
		if err := st.UpdateStatus(ctx, j1.ID, job.StatusDead); err != nil {
			t.Errorf("FAILED -> DEAD should be valid, got: %v", err)
		}
	})

	t.Run("Updated At being updated", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		st, err := job.NewRedisStore(client, ttl)
		if err != nil {
			t.Fatalf("NewRedisStore failed: %v", err)
		}

		j2 := testutils.NewTestJob("j2", 5, 0)
		if err := st.Save(ctx, j2); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Capture initial UpdatedAt
		initialUpdatedAt := j2.UpdatedAt

		// Wait for a short duration to ensure UpdatedAt will change
		time.Sleep(1 * time.Second)

		// Update status to "running"
		if err := st.UpdateStatus(ctx, j2.ID, job.StatusRunning); err != nil {
			t.Fatalf("Failed to update status: %v", err)
		}

		// Retrieve the job again to check UpdatedAt
		updatedJob, err := st.Get(ctx, j2.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve job: %v", err)
		}
		if updatedJob == nil {
			t.Fatalf("Get returned nil job without error")
		}

		if !updatedJob.UpdatedAt.After(initialUpdatedAt) {
			t.Errorf("Expected UpdatedAt to be updated, but it was not. Initial: %v, Updated: %v", initialUpdatedAt, updatedJob.UpdatedAt)
		}
	})
}
