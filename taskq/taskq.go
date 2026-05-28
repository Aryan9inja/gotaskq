package taskq

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	}

	queueManager := queue.NewQueueManager()
	if err := queueManager.Register(mainQueue); err != nil {
		return nil, fmt.Errorf("register default queue: %w", err)
	}

	registry := internalHandler.NewRegistry()
	retryEngine := retry.NewRetryEngine(jobStore, opts.MaxRetryDelay, dlqStore)
	workerPool := worker.NewWorkerPool(context.Background(), jobStore, registry, retryEngine, opts.NumWorkers)
	router := newRouter(jobStore, queueManager, snowflake.New(1), dlqStore, queueFactory, workerPool)

	return &Server{
		httpServer: &http.Server{
			Addr:    opts.Addr,
			Handler: router,
		},
		workerPool:      workerPool,
		handlers:        registry,
		redisClient:     redisClient,
		ownsRedisClient: ownsRedis,
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

// Start starts the worker pool and blocks while the HTTP server is running
func (s *Server) Start() error{
	if s == nil {
		return errors.New("taskq: server is nil")
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return errors.New("taskq: server is stopped")
	}

	if !s.workerStarted {
		s.workerPool.Start()
		s.workerStarted = true
	}
	s.mu.Unlock()

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
	if err := s.httpServer.Shutdown(ctx); err != nil && !errors.Is(err,http.ErrServerClosed) {
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