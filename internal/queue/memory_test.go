package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/queue"
	testutils "github.com/Aryan9inja/gotaskq/internal/testUtils"
)

func TestMemoryQueue(t *testing.T) {
	ctx := context.Background()

	t.Run("Enqueue and Dequeue Ordering", func(t *testing.T) {
		q := queue.NewMemoryQueue("test-queue")
		// Higher priority, but later run time
		j1 := testutils.NewTestJob("j1", 10, 100*time.Millisecond)

		// Lower priority, but earlier run time
		j2 := testutils.NewTestJob("j2", 5, 0)

		q.Enqueue(ctx, j1)
		q.Enqueue(ctx, j2)

		got, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Dequeue failed: %v", err)
		}
		if got.ID != "j2" {
			t.Fatalf("Expected j2 to be dequeued first, got: %s", got.ID)
		}

		got, err = q.Dequeue(ctx)
		if err == nil || err.Error() != "no jobs ready" {
			t.Fatalf("Expected 'no jobs ready' error, got: %v", err)
		}

		// Wait for j1 to be ready
		time.Sleep(105 * time.Millisecond)
		got, err = q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Dequeue j1 failed: %v", err)
		}
		if got.ID != "j1" {
			t.Fatalf("Expected j1 to be dequeued, got: %s", got.ID)
		}
	})

	t.Run("Priority Ordering", func(t *testing.T) {
		qx := queue.NewMemoryQueue("priority-test-queue")
		now := time.Now()

		j1 := testutils.NewTestJob("low", 1, 0)
		j1.RunAfter = now.Add(50 * time.Millisecond)
		j2 := testutils.NewTestJob("high", 10, 0)
		j2.RunAfter = now.Add(50 * time.Millisecond)

		qx.Enqueue(ctx, j1)
		qx.Enqueue(ctx, j2)

		// Wait for both to be ready
		time.Sleep(55 * time.Millisecond)

		got, err := qx.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Dequeue failed: %v", err)
		}
		if got.ID != "high" {
			t.Fatalf("Expected high priority job to be dequeued first, got: %s", got.ID)
		}
	})

	t.Run("Empty Queue", func(t *testing.T) {
		q := queue.NewMemoryQueue("empty-test-queue")
		_, err := q.Dequeue(ctx)
		if err == nil || err.Error() != "queue is empty" {
			t.Fatalf("Expected 'queue is empty' error, got: %v", err)
		}
	})

	t.Run("Len and Name", func(t *testing.T) {
		q := queue.NewMemoryQueue("len-name-test-queue")
		if q.Name() != "len-name-test-queue" {
			t.Fatalf("Expected queue name to be 'len-name-test-queue', got: %s", q.Name())
		}
		if q.Len() != 0 {
			t.Fatalf("Expected initial length to be 0, got: %d", q.Len())
		}

		q.Enqueue(ctx, testutils.NewTestJob("j1", 1, 0))
		q.Enqueue(ctx, testutils.NewTestJob("j2", 1, 0))
		if q.Len() != 2 {
			t.Fatalf("Expected length to be 2 after enqueuing 2 jobs, got: %d", q.Len())
		}
	})
}
