package worker

import (
	"context"
	"sync"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/handler"
	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/queue"
	"github.com/Aryan9inja/gotaskq/internal/retry"
)

type Pool struct {
	queue    queue.Queue
	store    job.Store
	registry *handler.Registry
	retry    *retry.RetryEngine // Not defined yet
	// dlq
	// metrics
	numWorkers int
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
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

			pool.processJob(job)
		}
	}
}

func (pool *Pool) processJob(j *job.Job) {
	// 1. Mark the job as running
	_ = pool.store.UpdateStatus(pool.ctx, j.ID, job.StatusRunning)

	defer func() {
		if r := recover(); r != nil {
			_ = pool.store.UpdateStatus(pool.ctx, j.ID, job.StatusFailed)
		}
	}()

	// 2. Get handler
	hand, ok := pool.registry.Get(j.Type)
	if !ok {
		_ = pool.store.UpdateStatus(pool.ctx, j.ID, job.StatusFailed)
		return
	}

	// 3. Execute the handler we got
	err := hand.Handle(pool.ctx, j)
	if err != nil {
		_ = pool.store.UpdateStatus(pool.ctx, j.ID, job.StatusFailed)
		return
	}

	// 4. Mark the job as done
	err = pool.store.UpdateStatus(pool.ctx, j.ID, job.StatusDone)
}
