package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/handler"
	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/queue"
	"github.com/Aryan9inja/gotaskq/internal/retry"
)

type HandlerGet interface {
	Get(jobType string) (handler.Handler, bool)
}

type Pool struct {
	queue    queue.Queue
	store    job.Store
	registry HandlerGet
	retry    retry.Engine
	// dlq
	// metrics
	numWorkers int
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewWorkerPool(parentCtx context.Context, q queue.Queue, st job.Store, registry HandlerGet, rtry retry.Engine, numWorkers int) *Pool {
	if numWorkers <= 0 {
		numWorkers = 1
	}

	if parentCtx == nil {
		parentCtx = context.Background()
	}

	ctx, cancel := context.WithCancel(parentCtx)

	return &Pool{
		queue:      q,
		store:      st,
		registry:   registry,
		retry:      rtry,
		numWorkers: numWorkers,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (pool *Pool) Start() {
	for i := 0; i < pool.numWorkers; i++ {
		pool.wg.Add(1)
		go pool.runWorker(i)
	}
}

func (pool *Pool) Stop() {
	pool.cancel()
	pool.wg.Wait()
}

func (pool *Pool) runWorker(id int) {
	defer pool.wg.Done()

	for {
		select {
		case <-pool.ctx.Done():
			return

		default:
			job, err := pool.queue.Dequeue(pool.ctx)
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if err := pool.processJob(job); err != nil {
				log.Printf("worker %d: failed to process job %s: %v", id, job.ID, err)
			}
		}
	}
}

func (pool *Pool) processJob(j *job.Job) (err error) {
	if j == nil {
		return errors.New("nil job")
	}

	// 1. Mark the job as running
	err = pool.store.UpdateStatus(pool.ctx, j.ID, job.StatusRunning)
	if err != nil {
		return fmt.Errorf("failed to mark job %s as running: %w", j.ID, err)
	}
	j.Status = job.StatusRunning

	defer func() {
		if r := recover(); r != nil {
			panicErr := fmt.Errorf("panic while processing job %s: %v", j.ID, r)
			statusErr := pool.store.UpdateStatus(pool.ctx, j.ID, job.StatusFailed)
			if statusErr != nil {
				err = errors.Join(panicErr, fmt.Errorf("failed to mark job %s as failed after panic: %w", j.ID, statusErr))
				return
			}
			j.Status = job.StatusFailed
			err = panicErr
		}
	}()

	// 2. Get handler
	hand, ok := pool.registry.Get(j.Type)
	if !ok {
		statusErr := pool.store.UpdateStatus(pool.ctx, j.ID, job.StatusFailed)
		if statusErr != nil {
			return errors.Join(
				fmt.Errorf("no handler registered for job type %s", j.Type),
				fmt.Errorf("failed to mark job %s as failed: %w", j.ID, statusErr),
			)
		}
		j.Status = job.StatusFailed

		return fmt.Errorf("no handler registered for job type %s", j.Type)
	}

	// 3. Execute the handler we got
	err = hand.Handle(pool.ctx, j)
	if err != nil {
		statusErr := pool.store.UpdateStatus(pool.ctx, j.ID, job.StatusFailed)
		if statusErr != nil {
			return errors.Join(
				fmt.Errorf("handler failed for job %s: %w", j.ID, err),
				fmt.Errorf("failed to mark job %s as failed: %w", j.ID, statusErr),
			)
		}
		j.Status = job.StatusFailed

		return fmt.Errorf("handler failed for job %s: %w", j.ID, err)
	}

	// 4. Mark the job as done
	err = pool.store.UpdateStatus(pool.ctx, j.ID, job.StatusDone)
	if err != nil {
		return fmt.Errorf("failed to mark job %s as done: %w", j.ID, err)
	}
	j.Status = job.StatusDone

	return nil
}
