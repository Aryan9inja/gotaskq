package retry

import (
	"context"
	"testing"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/job"
)

// Mocking job store functionality
type MockStore struct {
	saveFunc   func(ctx context.Context, job *job.Job) error
	updateFunc func(ctx context.Context, id string, status job.Status) error
}

func (m *MockStore) Save(ctx context.Context, job *job.Job) error {
	return m.saveFunc(ctx, job)
}
func (m *MockStore) Get(ctx context.Context, id string) (*job.Job, error) { return nil, nil }
func (m *MockStore) UpdateStatus(ctx context.Context, id string, status job.Status) error {
	return m.updateFunc(ctx, id, status)
}
func (m *MockStore) Delete(ctx context.Context, id string) error { return nil }

// Mocking queue functionality
type MockQueue struct {
	enqueueFunc func(ctx context.Context, job *job.Job) error
	nameFunc    func() string
}

func (q *MockQueue) Enqueue(ctx context.Context, job *job.Job) error {
	return q.enqueueFunc(ctx, job)
}
func (q *MockQueue) Dequeue(ctx context.Context) (job *job.Job, error error) { return nil, nil }
func (q *MockQueue) Len() int                                                { return 0 }
func (q *MockQueue) Name() string {
	return q.nameFunc()
}

// Mocking DLQ functionality
type MockDlq struct {
	saveFunc func(ctx context.Context, j *job.Job) error
}

func (dq *MockDlq) Save(ctx context.Context, j *job.Job) error {
	return dq.saveFunc(ctx, j)
}
func (dq *MockDlq) Get(ctx context.Context, id string) (*job.Job, error)      { return nil, nil }
func (dq *MockDlq) Delete(ctx context.Context, id string) error               { return nil }
func (dq *MockDlq) List(ctx context.Context, limit int64) ([]*job.Job, error) { return nil, nil }

func TestShouldRetry(t *testing.T) {
	cases := []struct {
		name string
		job  *job.Job
		want bool
	}{
		{
			name: "Should retry when retry count is less than maxRetries",
			job: &job.Job{
				RetryCount: 0,
				MaxRetries: 3,
			},
			want: true,
		},
		{
			name: "Should not retry when retry count equals maxRetries",
			job: &job.Job{
				RetryCount: 3,
				MaxRetries: 3,
			},
			want: false,
		},
		{
			name: "Should not retry when retry count exceeds maxRetries",
			job: &job.Job{
				RetryCount: 4,
				MaxRetries: 3,
			},
			want: false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := ShouldRetry(testCase.job); got != testCase.want {
				t.Errorf("ShouldRetry() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestNextDelay(t *testing.T) {
	engine := &RetryEngine{MaxDelay: 10 * time.Second}
	j := &job.Job{
		Delay:      1 * time.Second,
		RetryCount: 1,
	}

	delay := engine.NextDelay(j)

	// Check if delay is at least the exponential backoff (2s) and within jitter range (upto 2s + 0.4*2 = 2.8s)
	if delay < 2*time.Second || delay > 2800*time.Millisecond {
		t.Errorf("Next Delay() = %v, want range [2s, 2.8s]", delay)
	}

	// Check if delay > maxDelay
	j.RetryCount = 10 // 1 * 2^10 = 1024s
	delay = engine.NextDelay(j)
	if delay < 10*time.Second || delay > 14*time.Second {
		t.Errorf("NextDelay() result = (%v), want in range [10s, 14s]", delay)
	}
}

func TestHandleFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("Retry Success", func(t *testing.T) {
		j := &job.Job{
			ID:         "job-1",
			RetryCount: 0,
			MaxRetries: 3,
			Delay:      100 * time.Millisecond,
		}

		storeUpdated := false
		enqueued := false

		mockStore := &MockStore{
			updateFunc: func(ctx context.Context, id string, status job.Status) error {
				if id == "job-1" && status == job.StatusPending {
					storeUpdated = true
				}
				return nil
			},
		}

		mockQueue := &MockQueue{
			enqueueFunc: func(ctx context.Context, job *job.Job) error {
				if job.ID == "job-1" {
					enqueued = true
				}
				return nil
			},
			nameFunc: func() string { return "test-queue" },
		}

		engine := NewRetryEngine(mockStore, 10*time.Minute, nil)
		engine.HandleFailure(ctx, mockQueue, j)

		if j.RetryCount != 1 {
			t.Errorf("Expected retry count to be 1, got %d", j.RetryCount)
		}
		if !storeUpdated {
			t.Error("Job status was not updated in store")
		}
		if !enqueued {
			t.Error("Job was not re-enqueud")
		}
		if j.Status != job.StatusPending {
			t.Errorf("Expected status pending, got %s", j.Status)
		}
		if j.RunAfter.Before(time.Now()) {
			t.Error("Run After was not set in future")
		}
	})

	t.Run("Move to DLQ", func(t *testing.T) {
		j := &job.Job{
			ID:         "job-2",
			RetryCount: 3,
			MaxRetries: 3,
			Delay:      100 * time.Millisecond,
		}

		storeSaved := false
		storeUpdated := false
		dlqSaved := false

		mockStore := &MockStore{
			saveFunc: func(ctx context.Context, job *job.Job) error {
				if job.ID == "job-2" {
					storeSaved = true
				}
				return nil
			},
			updateFunc: func(ctx context.Context, id string, status job.Status) error {
				if id == "job-2" && status == job.StatusDead {
					storeUpdated = true
				}
				return nil
			},
		}

		mockDLQ := &MockDlq{
			saveFunc: func(ctx context.Context, j *job.Job) error {
				if j.ID == "job-2" {
					dlqSaved = true
				}
				return nil
			},
		}

		mockQueue := &MockQueue{
			nameFunc: func() string { return "test-queue" },
		}

		engine := NewRetryEngine(mockStore, 10*time.Minute, mockDLQ)
		engine.HandleFailure(ctx, mockQueue, j)

		if !storeSaved {
			t.Error("Job state was not saved in store")
		}
		if !storeUpdated {
			t.Error("Job status was not updated in store")
		}
		if !dlqSaved {
			t.Error("Job was not saved in dlq")
		}
		if j.Status != job.StatusDead {
			t.Errorf("Expected status dead, got %s", j.Status)
		}
		if j.Error != "Max retries exceeded" {
			t.Errorf("Expecetd error 'Max retries exceeded', got %q", j.Error)
		}
	})

	t.Run("Context Cancelled", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		j := &job.Job{ID: "job-3"}
		engine := NewRetryEngine(nil, 10*time.Minute, nil)
		engine.HandleFailure(cancelCtx, nil, j)

		if j.Error != "Context not found during handle failure" {
			t.Errorf("Expected context error, got %q", j.Error)
		}
	})
}
