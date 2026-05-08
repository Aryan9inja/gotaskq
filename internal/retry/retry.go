package retry

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/dlq"
	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/metrics"
	"github.com/Aryan9inja/gotaskq/internal/queue"
)

type Engine interface {
	HandleFailure(ctx context.Context, q queue.Queue, j *job.Job)
}

type RetryEngine struct {
	store    job.Store
	MaxDelay time.Duration
	dlq      dlq.DlqInterface
}

func NewRetryEngine(st job.Store, maxDelay time.Duration, dlqStore dlq.DlqInterface) *RetryEngine {
	return &RetryEngine{
		store:    st,
		MaxDelay: maxDelay,
		dlq:      dlqStore,
	}
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

func (engine *RetryEngine) HandleFailure(ctx context.Context, q queue.Queue, j *job.Job) {
	queueName := "unknown"
	if q != nil {
		queueName = q.Name()
	}

	select {
	case <-ctx.Done():
		err := ctx.Err()
		j.Error = "Context not found during handle failure"
		fmt.Printf("Handle failure: during contextCheck : %v", err)
		return
	default:
	}

	if ShouldRetry(j) {
		j.RetryCount++
		metrics.IncJobsRetried(queueName, j.Type)

		delay := engine.NextDelay(j)

		j.RunAfter = time.Now().Add(delay)

		err := engine.store.UpdateStatus(ctx, j.ID, job.StatusPending)
		if err != nil {
			j.Error = "Cannot update status to pending while retrying"
			fmt.Printf("Handle failure: updateStatus to pending : %v", err)
		} else {
			j.Status = job.StatusPending
		}

		if q != nil {
			err = q.Enqueue(ctx, j)
			if err != nil {
				j.Error = "Not able to enqueue the job"
				fmt.Printf("Handle failure: enqueue job : %v", err)
			}
		} else {
			fmt.Printf("Handle failure: queue is nil, cannot enqueue job %s", j.ID)
		}

		return
	}

	// Max retries exceeded
	j.Error = "Max retries exceeded"
	err := engine.store.Save(ctx, j)
	if err != nil {
		fmt.Printf("Handle failure: save dead job state: %v", err)
	}

	err = engine.store.UpdateStatus(ctx, j.ID, job.StatusDead)
	if err != nil {
		j.Error = "Cannot update status to dead"
		fmt.Printf("Handle failure: updateStatus to dead : %v", err)
	} else {
		j.Status = job.StatusDead
	}

	// Push into our dlq
	if engine.dlq == nil {
		fmt.Printf("Handle failure: dlq is nil, cannot persist dead job %s", j.ID)
		return
	}

	metrics.IncJobsDead(q.Name(), j.Type)

	err = engine.dlq.Save(ctx, j)
	if err != nil {
		fmt.Printf("Handle failure: save dead job to dlq: %v", err)
	}
}
