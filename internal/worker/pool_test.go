package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/handler"
	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/queue"
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

// Mocking job store
type MockStore struct {
	updateFunc func(ctx context.Context, id string, status job.Status) error
	deleteFunc func(ctx context.Context, id string) error
}

func (m *MockStore) Save(ctx context.Context, job *job.Job) error         { return nil }
func (m *MockStore) Get(ctx context.Context, id string) (*job.Job, error) { return nil, nil }
func (m *MockStore) UpdateStatus(ctx context.Context, id string, status job.Status) error {
	return m.updateFunc(ctx, id, status)
}
func (m *MockStore) Delete(ctx context.Context, id string) error {
	return m.deleteFunc(ctx, id)
}

// Mocking handler registry
type MockRegistry struct {
	handlers map[string]handler.Handler
}

func (r *MockRegistry) Get(jobType string) (handler.Handler, bool) {
	h, ok := r.handlers[jobType]
	return h, ok
}

// Mocking retry engine
type MockRetryEngine struct {
	handleFailureFunc func(ctx context.Context, q queue.Queue, j *job.Job)
}

func (re *MockRetryEngine) HandleFailure(ctx context.Context, q queue.Queue, j *job.Job) {
	re.handleFailureFunc(ctx, q, j)
}

// Mocking Handler
type MockHandler struct {
	handleFunc func(ctx context.Context, job *job.Job) error
}

func (mh *MockHandler) Handle(ctx context.Context, job *job.Job) error {
	return mh.handleFunc(ctx, job)
}

// Mocking Queue for control
type ManualQueue struct {
	name string
	jobs chan *job.Job
}

func (mq *ManualQueue) Enqueue(ctx context.Context, job *job.Job) error {
	mq.jobs <- job
	return nil
}
func (mq *ManualQueue) Dequeue(ctx context.Context) (job *job.Job, error error){
	select{
	case j := <-mq.jobs:
		return j, nil
	default:
		return nil, errors.New("empty queue")
	}
}
func (mq *ManualQueue) Len() int {
	return len(mq.jobs)
}
func (mq *ManualQueue) Name() string{
	return mq.name
}

func TestWorkerPoolProcessing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	t.Run("Process Job Successfully", func(t *testing.T) {
		var statusChange []job.Status
		var deleted bool
		var wg sync.WaitGroup
		wg.Add(1)

		mockStore := &MockStore{
			updateFunc: func(ctx context.Context, id string, status job.Status) error {
				statusChange = append(statusChange, status)
				return nil
			},
			deleteFunc: func(ctx context.Context, id string) error {
				deleted = true
				wg.Done()
				return nil
			},
		}

		mockHandler := &MockHandler{
			handleFunc: func(ctx context.Context, job *job.Job) error {
				return nil
			},
		}

		mockRegistry := &MockRegistry{
			handlers: map[string]handler.Handler{"test":mockHandler},
		}

		pool := NewWorkerPool(ctx, mockStore, mockRegistry, &MockRetryEngine{}, 1)
		q := &ManualQueue{
			name: "test-queue",
			jobs: make(chan *job.Job, 1),
		}
		pool.AddQueue(q)
		pool.Start()
		defer pool.Stop()

		j := &job.Job{ID: "job-1", Type: "test"}
		q.jobs <- j

		wg.Wait()

		if !deleted{
			t.Error("Job was not deleted after successfull processing")
		}

		// Expecting transition from running to done
		foundRunning := false
		foundDone := false
		for _, s := range statusChange{
			if s == job.StatusRunning {foundRunning = true}
			if s == job.StatusDone {foundDone = true}
		}
		if !foundRunning || !foundDone{
			t.Errorf("Missing status transition. Got %v", statusChange)
		}
	})

	t.Run("Handle error in handler", func(t *testing.T) {
		retryCalled := false
		var wg sync.WaitGroup
		wg.Add(1)

		retry := &MockRetryEngine{
			handleFailureFunc: func(ctx context.Context, q queue.Queue, j *job.Job) {
				retryCalled = true
				wg.Done()
			},
		}

		mockStore := &MockStore{
			updateFunc: func(ctx context.Context, id string, status job.Status) error {
				return nil
			},
		}

		mockHandler := &MockHandler{
			handleFunc: func(ctx context.Context, job *job.Job) error {
				return errors.New("failed")
			},
		}

		mockRegistry := &MockRegistry{
			handlers: map[string]handler.Handler{"test":mockHandler},
		}

		pool := NewWorkerPool(ctx, mockStore, mockRegistry, retry, 1)
		q := &ManualQueue{
			name: "test-queue",
			jobs: make(chan *job.Job, 1),
		}
		pool.AddQueue(q)
		pool.Start()
		defer pool.Stop()

		j := &job.Job{ID: "job-2", Type: "test"}
		q.jobs <- j

		wg.Wait()

		if !retryCalled{
			t.Error("Retry engine was not called after handler failure")
		}
	})

	t.Run("Handle panic in handler", func(t *testing.T) {
		retryCalled := false
		var wg sync.WaitGroup
		wg.Add(1)

		retry := &MockRetryEngine{
			handleFailureFunc: func(ctx context.Context, q queue.Queue, j *job.Job) {
				retryCalled = true
				wg.Done()
			},
		}

		mockStore := &MockStore{
			updateFunc: func(ctx context.Context, id string, status job.Status) error {
				return nil
			},
		}

		mockHandler := &MockHandler{
			handleFunc: func(ctx context.Context, job *job.Job) error {
				panic("boom")
			},
		}

		mockRegistry := &MockRegistry{
			handlers: map[string]handler.Handler{"test":mockHandler},
		}

		pool := NewWorkerPool(ctx, mockStore, mockRegistry, retry, 1)
		q := &ManualQueue{
			name: "test-queue",
			jobs: make(chan *job.Job, 1),
		}
		pool.AddQueue(q)
		pool.Start()
		defer pool.Stop()

		j := &job.Job{ID: "job-3", Type: "test"}
		q.jobs <- j

		wg.Wait()

		if !retryCalled{
			t.Error("Retry engine was not called after handler panic")
		}
	})
}

func TestRoundRobinPolling(t *testing.T) {
	pool := &Pool{
		queues: []queue.Queue{
			&ManualQueue{name: "q1"},
			&ManualQueue{name: "q2"},
			&ManualQueue{name: "q3"},
		},
	}

	qSnap1 := pool.queueSnapshot()
	if qSnap1[0].Name() != "q1"{
		t.Errorf("Expected first queue q1, got %q", qSnap1[0].Name())
	}

	qSnap2 := pool.queueSnapshot()
	if qSnap2[0].Name() != "q2"{
		t.Errorf("Expected first queue q2, got %q", qSnap2[0].Name())
	}

	qSnap3 := pool.queueSnapshot()
	if qSnap3[0].Name() != "q3"{
		t.Errorf("Expected first queue q3, got %q", qSnap3[0].Name())
	}

	qSnap4 := pool.queueSnapshot()
	if qSnap4[0].Name() != "q1"{
		t.Errorf("Expected wrap around to q1, got %q", qSnap4[0].Name())
	}
}