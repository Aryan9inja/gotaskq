package taskq

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/api/handlers"
	"github.com/Aryan9inja/gotaskq/internal/dlq"
	internalHandler "github.com/Aryan9inja/gotaskq/internal/handler"
	internalJob "github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/queue"
	"github.com/Aryan9inja/gotaskq/internal/retry"
	"github.com/Aryan9inja/gotaskq/internal/worker"
	"github.com/Aryan9inja/gotaskq/pkg/snowflake"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

const shutdownTimeout = 5 * time.Second

// Server owns the HTTP API's, queueBackend, handlerRegistry and workerPool
type Server struct {
	httpServer      *http.Server
	workerPool      *worker.Pool
	handlers        *internalHandler.Registry
	redisClient     redis.UniversalClient
	ownsRedisClient bool

	jobStore     internalJob.Store
	queueManager *queue.QueueManager
	idGenerator  *snowflake.Snowflake
	dlqStore     dlq.DlqInterface
	queueFactory queue.Factory

	mu            sync.Mutex
	workerStarted bool
	stopped       bool
}

// New creates and configures a new taskq server with given options
// It wire all internal components but will not start processing until Start is called
func New(opts Options) (*Server, error) {
	opts = normalizeOptions(opts)

	var (
		jobStore     internalJob.Store
		mainQueue    queue.Queue
		dlqStore     dlq.DlqInterface
		queueFactory queue.Factory
		redisClient  redis.UniversalClient
		ownsRedis    bool
	)

	if opts.UseRedis || opts.RedisClient != nil {
		client, ownsClient, err := redisClientFromOptions(opts)
		if err != nil {
			return nil, err
		}

		redisStore, err := internalJob.NewRedisStore(client, time.Duration(opts.TTL)*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("create redis job store: %w", err)
		}

		redisFactory, err := queue.NewRedisFactory(client)
		if err != nil {
			return nil, fmt.Errorf("create redis queue factory: %w", err)
		}

		redisQueue, err := redisFactory.New(opts.DefaultQueueName)
		if err != nil {
			return nil, fmt.Errorf("create redis queue: %w", err)
		}

		redisDLQ, err := dlq.NewRedisDlq(client)
		if err != nil {
			return nil, fmt.Errorf("create redis dlq: %w", err)
		}

		jobStore = redisStore
		mainQueue = redisQueue
		dlqStore = redisDLQ
		queueFactory = redisFactory
		redisClient = client
		ownsRedis = ownsClient
	} else {
		memoryFactory := queue.NewMemoryFactory()
		memoryQueue, err := memoryFactory.New(opts.DefaultQueueName)
		if err != nil {
			return nil, fmt.Errorf("create memory queue: %w", err)
		}

		jobStore = internalJob.NewMemoryStore(time.Duration(opts.TTL) * time.Minute)
		mainQueue = memoryQueue
		queueFactory = memoryFactory
		dlqStore = dlq.NewMemoryDlq()
	}

	queueManager := queue.NewQueueManager()
	if err := queueManager.Register(mainQueue); err != nil {
		return nil, fmt.Errorf("register default queue: %w", err)
	}

	registry := internalHandler.NewRegistry()
	retryEngine := retry.NewRetryEngine(jobStore, opts.MaxRetryDelay, dlqStore)
	workerPool := worker.NewWorkerPool(context.Background(), jobStore, registry, retryEngine, opts.NumWorkers)
	workerPool.AddQueue(mainQueue)

	idGenerator := snowflake.New(1)
	router := newRouter(jobStore, queueManager, idGenerator, dlqStore, queueFactory, workerPool)

	return &Server{
		httpServer: &http.Server{
			Addr:    opts.Addr,
			Handler: router,
		},
		workerPool:      workerPool,
		handlers:        registry,
		redisClient:     redisClient,
		ownsRedisClient: ownsRedis,
		jobStore:        jobStore,
		queueManager:    queueManager,
		idGenerator:     idGenerator,
		dlqStore:        dlqStore,
		queueFactory:    queueFactory,
	}, nil
}

func newRouter(
	store internalJob.Store,
	queueManager *queue.QueueManager,
	idGenerator *snowflake.Snowflake,
	dlqStore dlq.DlqInterface,
	queueFactory queue.Factory,
	registrar queue.Registrar,
) http.Handler {
	router := chi.NewRouter()
	handler := handlers.New(store, queueManager, idGenerator, dlqStore, queueFactory, registrar)

	router.Handle("/metrics", promhttp.Handler())
	router.Post("/jobs", handler.CreateJob)
	router.Get("/jobs/{id}", handler.GetJob)
	router.Post("/{queue}/jobs", handler.CreateJobOnQueue)
	router.Get("/dlq", handler.ListDeadJobs)
	router.Post("/dlq/{id}/replay", handler.ReplayDeadJob)
	router.Delete("/dlq/{id}", handler.DeleteDeadJob)
	router.Post("/queue/{name}", handler.CreateNewQueue)
	router.Get("/queue", handler.ListQueues)

	return router
}

// Register binds a job type to a handler
func (s *Server) Register(jobType string, handler Handler) error {
	if s == nil {
		return errors.New("taskq: server is nil")
	}

	if strings.TrimSpace(jobType) == "" {
		return errors.New("taskq: job type cannot be empty")
	}

	if handler == nil {
		return errors.New("taskq: handler cannot be nil")
	}

	s.handlers.Register(jobType, handlerAdapter{handler: handler})
	return nil
}

// RegisterFunc binds a job type to a handler function
func (s *Server) RegisterFunc(jobType string, handlerFunc func(ctx context.Context, job *Job) error) error {
	if handlerFunc == nil {
		return errors.New("taskq: handler function cannot be nil")
	}
	return s.Register(jobType, HandlerFunc(handlerFunc))
}

// StartWorkers starts only the background worker pool
func (s *Server) StartWorkers() error {
	if s == nil {
		return errors.New("taskq: server is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return errors.New("taskq: server is stopped")
	}

	if !s.workerStarted {
		s.workerPool.Start()
		s.workerStarted = true
	}

	return nil
}

// Start starts the worker pool and blocks while the HTTP server is running
func (s *Server) Start() error {
	if s == nil {
		return errors.New("taskq: server is nil")
	}

	if err := s.StartWorkers(); err != nil {
		return err
	}

	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	if err != nil {
		_ = s.Stop()
	}

	return err
}

// Stop gracefully shuts down the HTTP server and worker pool
func (s *Server) Stop() error {
	if s == nil {
		return errors.New("taskq: server is nil")
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}

	s.stopped = true
	workerStarted := s.workerStarted
	s.workerStarted = false
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	var stopErr error
	if err := s.httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		stopErr = errors.Join(stopErr, err)
	}

	if workerStarted {
		s.workerPool.Stop()
	}

	if s.ownsRedisClient && s.redisClient != nil {
		if err := s.redisClient.Close(); err != nil {
			stopErr = errors.Join(stopErr, err)
		}
	}

	return stopErr
}

// Enqueue creates and enqueues a new job on the default queue
func (s *Server) Enqueue(ctx context.Context, opts JobOptions) (*Job, error) {
	if s == nil {
		return nil, errors.New("taskq: server is nil")
	}

	q, err := s.queueManager.DefaultQueue()
	if err != nil {
		return nil, fmt.Errorf("taskq: failed to get default queue: %w", err)
	}

	return s.enqueueOnQueue(ctx, q, opts)
}

// EnqueueOnQueue creates and enqueues a new job on the specified queue
func (s *Server) EnqueueOnQueue(ctx context.Context, queueName string, opts JobOptions) (*Job, error) {
	if s == nil {
		return nil, errors.New("taskq: server is nil")
	}

	q, err := s.queueManager.Get(queueName)
	if err != nil {
		return nil, fmt.Errorf("taskq: failed to get queue %s: %w", queueName, err)
	}

	return s.enqueueOnQueue(ctx, q, opts)
}

func (s *Server) enqueueOnQueue(ctx context.Context, q queue.Queue, opts JobOptions) (*Job, error) {
	if opts.Type == "" {
		return nil, errors.New("taskq: job type is required")
	}

	if opts.Priority < 0 || opts.Priority > 10 {
		return nil, errors.New("taskq: priority must be between 0 and 10")
	}

	now := time.Now()
	id := strconv.FormatInt(s.idGenerator.NextID(), 10)

	internalJobObj := &internalJob.Job{
		ID:         id,
		Type:       opts.Type,
		Payload:    opts.Payload,
		Priority:   opts.Priority,
		Status:     internalJob.StatusPending,
		MaxRetries: opts.MaxRetries,
		CreatedAt:  now,
		UpdatedAt:  now,
		Delay:      opts.Delay,
	}

	if opts.Delay > 0 {
		internalJobObj.RunAfter = now.Add(opts.Delay)
	}

	if err := s.jobStore.Save(ctx, internalJobObj); err != nil {
		return nil, fmt.Errorf("taskq: failed to save job: %w", err)
	}

	if err := q.Enqueue(ctx, internalJobObj); err != nil {
		return nil, fmt.Errorf("taskq: failed to enqueue job: %w", err)
	}

	return wrapJob(internalJobObj), nil
}

// GetJob retrieves a job by its ID
func (s *Server) GetJob(ctx context.Context, id string) (*Job, error) {
	if s == nil {
		return nil, errors.New("taskq: server is nil")
	}

	internalJobObj, err := s.jobStore.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("taskq: failed to get job %s: %w", id, err)
	}

	return wrapJob(internalJobObj), nil
}

// ListQueues returns a list of all registered queue names
func (s *Server) ListQueues() []string {
	if s == nil {
		return nil
	}
	return s.queueManager.ListNames()
}

// CreateQueue creates and registers a new queue
func (s *Server) CreateQueue(name string) error {
	if s == nil {
		return errors.New("taskq: server is nil")
	}

	q, err := s.queueFactory.New(name)
	if err != nil {
		return fmt.Errorf("taskq: failed to create queue %s: %w", name, err)
	}

	if err := s.queueManager.Register(q); err != nil {
		return fmt.Errorf("taskq: failed to register queue %s: %w", name, err)
	}

	s.workerPool.AddQueue(q)

	return nil
}

// ListDeadJobs returns a list of dead jobs
func (s *Server) ListDeadJobs(ctx context.Context, limit int64) ([]*Job, error) {
	if s == nil {
		return nil, errors.New("taskq: server is nil")
	}

	if s.dlqStore == nil {
		return nil, errors.New("taskq: dlq store is not configured")
	}

	internalJobs, err := s.dlqStore.List(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("taskq: failed to list dead jobs: %w", err)
	}

	jobs := make([]*Job, len(internalJobs))
	for i, j := range internalJobs {
		jobs[i] = wrapJob(j)
	}

	return jobs, nil
}

// ReplayDeadJob moves a job from DLQ back to the default queue
func (s *Server) ReplayDeadJob(ctx context.Context, id string) (*Job, error) {
	if s == nil {
		return nil, errors.New("taskq: server is nil")
	}

	if s.dlqStore == nil {
		return nil, errors.New("taskq: dlq store is not configured")
	}

	q, err := s.queueManager.DefaultQueue()
	if err != nil {
		return nil, fmt.Errorf("taskq: failed to get default queue: %w", err)
	}

	deadJob, err := s.dlqStore.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("taskq: failed to get dead job %s: %w", id, err)
	}

	now := time.Now()
	deadJob.Status = internalJob.StatusPending
	deadJob.UpdatedAt = now
	deadJob.Error = ""
	deadJob.RunAfter = now

	if err := s.jobStore.Save(ctx, deadJob); err != nil {
		return nil, fmt.Errorf("taskq: failed to save replayed job: %w", err)
	}

	if err := q.Enqueue(ctx, deadJob); err != nil {
		return nil, fmt.Errorf("taskq: failed to enqueue replayed job: %w", err)
	}

	if err := s.dlqStore.Delete(ctx, id); err != nil {
		log.Printf("taskq: failed to delete job %s from DLQ after replay: %v", id, err)
	}

	return wrapJob(deadJob), nil
}

// DeleteDeadJob removes a job from the DLQ
func (s *Server) DeleteDeadJob(ctx context.Context, id string) error {
	if s == nil {
		return errors.New("taskq: server is nil")
	}

	if s.dlqStore == nil {
		return errors.New("taskq: dlq store is not configured")
	}

	if err := s.dlqStore.Delete(ctx, id); err != nil {
		return fmt.Errorf("taskq: failed to delete dead job %s: %w", id, err)
	}

	return nil
}
