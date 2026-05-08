package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/job"
)

type countingQueue struct {
	name    string
	dequeue atomic.Int64
}

func (q *countingQueue) Enqueue(ctx context.Context, job *job.Job) error {
	return nil
}

func (q *countingQueue) Dequeue(ctx context.Context) (job *job.Job, error error) {
	q.dequeue.Add(1)
	return nil, errors.New("queue is empty")
}

func (q *countingQueue) Len() int {
	return 0
}

func (q *countingQueue) Name() string {
	return q.name
}

func TestAddQueueDoesNotStartWorkerBeforeStartCalled(t *testing.T) {
	pool := NewWorkerPool(context.Background(), nil, nil, nil, 2)
	q := &countingQueue{name: "test"}

	pool.AddQueue(q)
	time.Sleep(25 * time.Millisecond)

	if got := q.dequeue.Load(); got != 0 {
		t.Fatalf("expected no dequeue attempt before Start call, got %d", got)
	}

	pool.Start()
	t.Cleanup(pool.Stop)

	deadline := time.Now().Add(250 * time.Millisecond)
	for q.dequeue.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	if got := q.dequeue.Load(); got == 0 {
		t.Fatal("expected dequeue attempt after Start call")
	}
}
