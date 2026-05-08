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
	"github.com/Aryan9inja/gotaskq/internal/metrics"
	"github.com/Aryan9inja/gotaskq/internal/queue"
	"github.com/Aryan9inja/gotaskq/internal/retry"
)

type HandlerGet interface {
	Get(jobType string) (handler.Handler, bool)
}

type Pool struct {
	store      job.Store
	registry   HandlerGet
	retry      retry.Engine
	numWorkers int
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc

	mu sync.Mutex

	// queues + cursor implements round robin polling across registered queues
	queues     []queue.Queue
	queueNames map[string]struct{}
	cursor     int

	// tells us if pool of worker has been initiated
	started bool

	// wakeCh makes workers to stop waiting when new work or notifications come
	wakeCh chan struct{}

	// functions to cancel pub/sub per queue
	unsubscribe []func()
}

func NewWorkerPool(parentCtx context.Context, st job.Store, registry HandlerGet, rtry retry.Engine, numWorkers int) *Pool {
	if numWorkers <= 0 {
		numWorkers = 1
	}

	if parentCtx == nil {
		parentCtx = context.Background()
	}

	ctx, cancel := context.WithCancel(parentCtx)

	return &Pool{
		store:      st,
		registry:   registry,
		retry:      rtry,
		numWorkers: numWorkers,
		ctx:        ctx,
		cancel:     cancel,
		queues:     make([]queue.Queue, 0),
		queueNames: make(map[string]struct{}),
		wakeCh:     make(chan struct{}, 1),
	}
}

func (pool *Pool) AddQueue(q queue.Queue) {
	if q == nil {
		log.Print("queue to add is nil")
		return
	}

	pool.mu.Lock()
	if _, exists := pool.queueNames[q.Name()]; exists {
		log.Print("queue already exist in worker pool")
		pool.mu.Unlock()
		return
	}

	pool.queues = append(pool.queues, q)
	pool.queueNames[q.Name()] = struct{}{}
	pool.mu.Unlock()

	if notifier, ok := q.(queue.Notifications); ok {
		ch, cancel, err := notifier.SubscribeNotifications(pool.ctx)
		if err != nil {
			log.Printf("queue %s: queue notification subscribe failed: %v", q.Name(), err)
		} else {
			pool.mu.Lock()
			pool.unsubscribe = append(pool.unsubscribe, cancel)
			pool.mu.Unlock()

			pool.wg.Add(1)
			go pool.forwardNotifications(ch)
		}
	}

	pool.wakeWorkers()
}

func (pool *Pool) wakeWorkers() {
	select {
	case pool.wakeCh <- struct{}{}:
	default:
	}
}

func (pool *Pool) forwardNotifications(ch <-chan struct{}) {
	defer pool.wg.Done()

	for {
		select {
		case <-pool.ctx.Done():
			return
		case _, ok := <-ch:
			if !ok {
				return
			}
			pool.wakeWorkers()
		}
	}
}

func (pool *Pool) Start() {
	pool.mu.Lock()
	if pool.started {
		pool.mu.Unlock()
		return
	}
	pool.started = true
	pool.mu.Unlock()

	for i := 0; i < pool.numWorkers; i++ {
		pool.wg.Add(1)
		go pool.runWorker(i)
	}

	pool.wakeWorkers()
}

func (pool *Pool) Stop() {
	pool.cancel()

	pool.mu.Lock()
	for _, unsubscribe := range pool.unsubscribe {
		unsubscribe()
	}
	pool.mu.Unlock()

	pool.wg.Wait()
}

func (pool *Pool) queueSnapshot() []queue.Queue {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	count := len(pool.queues)
	if count == 0 {
		return nil
	}

	start := pool.cursor % count
	pool.cursor = (pool.cursor + 1) % count

	queues := make([]queue.Queue, 0, count)
	for offset := range count {
		queues = append(queues, pool.queues[(start+offset)%count])
	}

	return queues
}

func (pool *Pool) dequeueReadyJobs() (queue.Queue, *job.Job, bool) {
	for _, q := range pool.queueSnapshot() {
		deqeueudJob, err := q.Dequeue(pool.ctx)
		if err == nil {
			return q, deqeueudJob, true
		}
	}
	return nil, nil, false
}

func (pool *Pool) waitForWork() {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()

	select {
	case <-pool.ctx.Done():
	case <-pool.wakeCh:
	case <-timer.C:
	}
}

func (pool *Pool) runWorker(id int) {
	defer pool.wg.Done()
	metrics.IncActiveWorkers()
	defer metrics.DecActiveWorkers()

	for {
		select {
		case <-pool.ctx.Done():
			return

		default:
			q, dequeuedJob, ok := pool.dequeueReadyJobs()
			if !ok {
				pool.waitForWork()
				continue
			}

			if err := pool.processJob(q, dequeuedJob); err != nil {
				log.Printf("worker %d for queue %s: failed to process job %s: %v", id, q.Name(), dequeuedJob.ID, err)
			}
		}
	}
}

func (pool *Pool) processJob(q queue.Queue, j *job.Job) (err error) {
	queueName := "unknown"
	if q != nil {
		queueName = q.Name()
	}

	jobType := "unknown"
	if j != nil {
		jobType = j.Type
	}

	start := time.Now()
	status := "failed"

	defer func() {
		metrics.ObserveJobDuration(queueName, jobType, time.Since(start))
		metrics.IncJobsProcessed(queueName, jobType, status)
	}()

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
			pool.retry.HandleFailure(pool.ctx, q, j)

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
		pool.retry.HandleFailure(pool.ctx, q, j)

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
		pool.retry.HandleFailure(pool.ctx, q, j)

		return fmt.Errorf("handler failed for job %s: %w", j.ID, err)
	}

	// 4. Mark the job as done
	err = pool.store.UpdateStatus(pool.ctx, j.ID, job.StatusDone)
	if err != nil {
		return fmt.Errorf("failed to mark job %s as done: %w", j.ID, err)
	}
	j.Status = job.StatusDone
	status = "done"

	// 5. Delete job after completion
	if delErr := pool.store.Delete(pool.ctx, j.ID); delErr != nil {
		log.Printf("failed to delete job %s from store: %v", j.ID, delErr)
	}

	return nil
}
