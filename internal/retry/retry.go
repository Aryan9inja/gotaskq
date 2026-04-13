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
	store job.Store
	queue queue.Queue
	// BaseDelay time.Duration
	MaxDelay time.Duration
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

func (engine *RetryEngine) HandleFailure(j *job.Job) {
	if ShouldRetry(j) {
		j.RetryCount++

		delay := engine.NextDelay(j)

		j.RunAfter = time.Now().Add(delay)

		err := engine.store.UpdateStatus(context.Background(), j.ID, job.StatusPending)
		if err != nil {
			fmt.Printf("Handle failure: %v", err)
		}

		engine.queue.Enequeue(context.Background(), j)

		return
	}

	// Max retries exceeded
	engine.store.UpdateStatus(context.Background(), j.ID, job.StatusDead)

	// Push into our dlq
	// TODO: push to DLQ
}
