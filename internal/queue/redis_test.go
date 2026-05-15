package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/queue"
	testutils "github.com/Aryan9inja/gotaskq/internal/testUtils"
)

func TestRedisQueue(t *testing.T) {
	client := testutils.GetRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	t.Run("Enqueue and Dequeue", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		queue, err := queue.NewRedisQueue("test-queue", client)
		if err != nil {
			t.Fatalf("Failed to create Redis queue: %v", err)
		}

		j1 := testutils.NewTestJob("j1", 5, 0)
		if err := queue.Enqueue(ctx, j1); err != nil {
			t.Fatalf("Failed to enqueue job: %v", err)
		}

		if queue.Len() != 1 {
			t.Errorf("Expected queue length 1, got %d", queue.Len())
		}

		dequeuedJob, err := queue.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Failed to dequeue job: %v", err)
		}

		if dequeuedJob.ID != j1.ID {
			t.Errorf("Expected job ID %s, got %s", j1.ID, dequeuedJob.ID)
		}

		if queue.Len() != 0 {
			t.Errorf("Expected queue length 0 after dequeue, got %d", queue.Len())
		}
	})

	t.Run("Priority and Ordering", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		queue, _ := queue.NewRedisQueue("test-queue", client)

		now := time.Now()
		runTime := now.Add(100 * time.Millisecond)

		j1 := testutils.NewTestJob("j1", 5, 0)
		j1.RunAfter = runTime
		j1.CreatedAt = now

		j2 := testutils.NewTestJob("j2", 10, 0)
		j2.RunAfter = runTime
		j2.CreatedAt = now

		queue.Enqueue(ctx, j1)
		queue.Enqueue(ctx, j2)

		// Wait for jobs to be ready
		time.Sleep(150 * time.Millisecond)

		got1, err := queue.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Dequeue 1 failed: %v", err)
		}
		if got1.ID != j2.ID {
			t.Errorf("Expected job ID %s, got %s", j2.ID, got1.ID)
		}

		got2, err := queue.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Dequeue 2 failed: %v", err)
		}
		if got2.ID != j1.ID {
			t.Errorf("Expected job ID %s, got %s", j1.ID, got2.ID)
		}
	})

	t.Run("Delay and RunAfter", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		queue, _ := queue.NewRedisQueue("test-queue", client)
		
		j1 := testutils.NewTestJob("j1", 5, 200*time.Millisecond)
		queue.Enqueue(ctx, j1)

		_,err := queue.Dequeue(ctx)
		if err == nil || err.Error() != "no jobs ready" {
			t.Errorf("Expected 'no jobs ready' error, got %v", err)
		}

		time.Sleep(250 * time.Millisecond)
		got, err := queue.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Failed to dequeue job after delay: %v", err)
		}
		if got.ID != j1.ID {
			t.Errorf("Expected job ID %s, got %s", j1.ID, got.ID)
		}
	})

	t.Run("Notifications", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		queue, _ := queue.NewRedisQueue("test-queue", client)

		notifyCh, unsubscribe, err := queue.SubscribeNotifications(ctx)
		if err != nil {
			t.Fatalf("Failed to subscribe to notifications: %v", err)
		}
		defer unsubscribe()

		j1 := testutils.NewTestJob("j1", 5, 0)
		go func() {
			if err := queue.Enqueue(ctx, j1); err != nil {
				t.Errorf("Failed to enqueue job: %v", err)
			}
		}()

		select {
		case <-notifyCh:
		case <-time.After(500 * time.Millisecond):
			t.Error("Expected notification but timed out")
		}
	})

	t.Run("Empty Queue Dequeue", func(t *testing.T) {
		testutils.ClearRedis(t, client)
		queue, _ := queue.NewRedisQueue("test-queue", client)

		_, err := queue.Dequeue(ctx)
		if err == nil || err.Error() != "queue is empty" {
			t.Errorf("Expected 'queue is empty' error, got %v", err)
		}
	})
}