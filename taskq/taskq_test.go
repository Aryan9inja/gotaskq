package taskq

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestServer_Embeddable(t *testing.T) {
	// Initialize server with memory backend
	server, err := New(Options{
		NumWorkers: 2,
	})
	if err != nil {
		t.Fatalf("failed to create taskq server: %v", err)
	}

	var (
		processedCount int
		mu             sync.Mutex
		wg             sync.WaitGroup
	)

	// Register a job handler
	wg.Add(2)
	err = server.RegisterFunc("test-job", func(ctx context.Context, job *Job) error {
		mu.Lock()
		processedCount++
		mu.Unlock()
		wg.Done()
		return nil
	})
	if err != nil {
		t.Fatalf("failed to register handler: %v", err)
	}

	// Start only workers
	if err := server.StartWorkers(); err != nil {
		t.Fatalf("failed to start workers: %v", err)
	}

	// Enqueue jobs directly
	ctx := context.Background()
	payload, _ := json.Marshal(map[string]string{"foo": "bar"})

	_, err = server.Enqueue(ctx, JobOptions{
		Type:    "test-job",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("failed to enqueue job 1: %v", err)
	}

	_, err = server.Enqueue(ctx, JobOptions{
		Type:    "test-job",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("failed to enqueue job 2: %v", err)
	}

	// Wait for jobs to be processed
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for jobs to be processed")
	}

	if processedCount != 2 {
		t.Errorf("expected 2 processed jobs, got %d", processedCount)
	}

	// Verify ListQueues
	queues := server.ListQueues()
	foundDefault := slices.Contains(queues, "default")
	if !foundDefault {
		t.Error("default queue not found in ListQueues")
	}

	// Verify GetJob
	// We'll enqueue one more and get it
	job, err := server.Enqueue(ctx, JobOptions{
		Type: "test-job",
	})
	if err != nil {
		t.Fatalf("failed to enqueue job for GetJob: %v", err)
	}

	retrievedJob, err := server.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("failed to get job: %v", err)
	}
	if retrievedJob.ID != job.ID {
		t.Errorf("expected job ID %s, got %s", job.ID, retrievedJob.ID)
	}

	// Stop
	if err := server.Stop(); err != nil {
		t.Fatalf("failed to stop server: %v", err)
	}
}

func TestServer_MemoryDLQ(t *testing.T) {
	// Initialize server with memory backend and 0 retries to push to DLQ immediately on failure
	server, err := New(Options{
		NumWorkers: 1,
	})
	if err != nil {
		t.Fatalf("failed to create taskq server: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	// Register a job handler that always fails
	err = server.RegisterFunc("fail-job", func(ctx context.Context, job *Job) error {
		defer wg.Done()
		return errors.New("intentional failure")
	})
	if err != nil {
		t.Fatalf("failed to register handler: %v", err)
	}

	if err := server.StartWorkers(); err != nil {
		t.Fatalf("failed to start workers: %v", err)
	}

	ctx := context.Background()
	job, err := server.Enqueue(ctx, JobOptions{
		Type:       "fail-job",
		MaxRetries: 0, // Should go to DLQ after 1st failure
	})
	if err != nil {
		t.Fatalf("failed to enqueue job: %v", err)
	}

	// Wait for job to fail
	wg.Wait()

	// Give it a tiny bit of time to move to DLQ
	time.Sleep(100 * time.Millisecond)

	// In a real scenario, we might want a way to list DLQ from Server too
	// Since I didn't add ListDLQ to Server yet, I'll just check if I can get it if I knew the ID
	// Actually, the user might want a way to interact with DLQ from Server too.
	// But for now, let's just verify it's working by checking the job status if we can.

	retrievedJob, err := server.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("failed to get job: %v", err)
	}

	if retrievedJob.Status != StatusDead {
		t.Errorf("expected job status DEAD, got %s", retrievedJob.Status)
	}

	// Verify ListDeadJobs
	deadJobs, err := server.ListDeadJobs(ctx, 10)
	if err != nil {
		t.Fatalf("failed to list dead jobs: %v", err)
	}
	if len(deadJobs) != 1 {
		t.Errorf("expected 1 dead job, got %d", len(deadJobs))
	}

	// Verify ReplayDeadJob
	wg.Add(1)
	// Change handler to succeed this time
	server.RegisterFunc("fail-job", func(ctx context.Context, job *Job) error {
		defer wg.Done()
		return nil
	})

	replayedJob, err := server.ReplayDeadJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("failed to replay dead job: %v", err)
	}
	if replayedJob.Status != StatusPending {
		t.Errorf("expected replayed job status PENDING, got %s", replayedJob.Status)
	}

	// Wait for replayed job to succeed
	wg.Wait()

	// Verify job is no longer in DLQ
	deadJobs, _ = server.ListDeadJobs(ctx, 10)
	if len(deadJobs) != 0 {
		t.Errorf("expected 0 dead jobs after replay, got %d", len(deadJobs))
	}

	// Clean up
	server.Stop()
}
