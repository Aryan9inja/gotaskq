package retry

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/queue"
)

type RetryEngine struct {
	store    job.Store
	queue    queue.Queue
	MaxDelay time.Duration
}

type Engine interface{
	HandleFailure(ctx context.Context, j *job.Job)
}

func ShouldRetry(j *job.Job) bool {
	return j.RetryCount < j.MaxRetries
}

func (engine *RetryEngine) NextDelay(j *job.Job) time.Duration {
	// Exponential backoff delay
	// delay = BaseDelay * 2^RetryCount + random_jitter
	delay := j.Delay * (1 << j.RetryCount)

	if delay > engine.MaxDelay {
		delay = engine.MaxDelay
	}

	jitter := time.Duration(rand.Float64() * 0.4 * float64(delay))

	return delay + jitter
}

func (engine *RetryEngine) HandleFailure(ctx context.Context, j *job.Job) {
	select {
	case <-ctx.Done():
		err := ctx.Err()
		j.Error="Context not found during handle failure"
		fmt.Printf("Handle failure: during contextCheck : %v", err)
		return
	default:
	}

	if ShouldRetry(j) {
		j.RetryCount++

		delay := engine.NextDelay(j)

		j.RunAfter = time.Now().Add(delay)

		err := engine.store.UpdateStatus(ctx, j.ID, job.StatusPending)
		if err != nil {
			j.Error = "Cannot update status to pending while retrying"
			fmt.Printf("Handle failure: updateStatus to pending : %v", err)
		}

		err = engine.queue.Enqueue(ctx, j)
		if err != nil {
			j.Error = "Not able to enqueue the job"
			fmt.Printf("Handle failure: enqueue job : %v", err)
		}

		return
	}

	// Max retries exceeded
	err := engine.store.UpdateStatus(ctx, j.ID, job.StatusDead)
	if err != nil {
		j.Error = "Cannot update status to dead"
		fmt.Printf("Handle failure: updateStatus to dead : %v", err)
	}

	// Push into our dlq
	// TODO: push to DLQ
}
