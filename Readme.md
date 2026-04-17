# GoTaskQ - Distributed Job Queue System in Go

## 1. Project Overview
GoTaskQ is a distibuted job queue system built in Go. Clients submit jobs via HTTP, and a pool of goroutines (workers) process them asynchronously with gaurantee around reliablity, ordering and failure handling.

### Tech Stack
- **Go**: Core language for server and worker implementation.
- **Chi**: Lightweight HTTP router for API endpoints.
- **Redis**: Primary storage for queues, job states, and Pub/Sub notifications.
- **In-Memory Priority Heap**: For fast job scheduling in development mode.
- **Snowflake IDs**: For unique, time-ordered job identifiers.
- **Prometheus**: For metrics and observability.
- **Go Routines & Channels**: For concurrent worker processing and coordination.

## 2. Folder Structure
```
gotaskq/
├── cmd/
│   └── server/
│       └── main.go          # entry point, wires everything together
├── internal/
│   ├── api/
│   │   ├── server.go        # HTTP server setup, middleware, route registration
│   │   ├── handlers/
│   │   │   ├── jobs.go      # POST /jobs, GET /jobs/:id, DELETE /jobs/:id
│   │   │   ├── queues.go    # GET /queues/:name/stats
│   │   │   └── dlq.go       # GET /dlq, POST /dlq/:id/replay
│   │   └── middleware/
│   │       └── logger.go    # request logging middleware
│   ├── queue/
│   │   ├── queue.go         # Queue interface definition
│   │   ├── memory.go        # in-memory priority heap implementation
│   │   ├── redis.go         # redis sorted set implementation
│   │   └── manager.go       # QueueManager — manages named queues
│   ├── worker/
│   │   ├── pool.go          # WorkerPool — goroutine pool, dispatch loop
│   │   └── worker.go        # individual worker logic, job execution
│   ├── job/
│   │   ├── job.go           # Job struct, Status constants, JobOptions
│   │   └── store.go         # JobStore interface + implementations
│   ├── retry/
│   │   └── retry.go         # RetryEngine, backoff calculation, jitter
│   ├── dlq/
│   │   └── dlq.go           # DeadLetterQueue, store + replay logic
│   ├── handler/
│   │   └── handler.go       # JobHandler interface, HandlerRegistry
│   └── metrics/
│       └── metrics.go       # Prometheus metrics definitions + helpers
├── pkg/
│   └── snowflake/
│       └── snowflake.go     # Snowflake ID generator (reusable, no internal deps)
├── config/
│   └── config.go            # Config struct, load from env
├── go.mod
├── go.sum
└── README.md
```

## 3. Key Design Decisions
| Decision | What I chose | Why |
| --- | --- | --- |
| Queue storage | Redis Sorted Set | Score = priority + timestamp, O(log n) dequeue |
| Job IDs | Snowflake | Time-ordered, no DB roundtrip, distributed-safe |
| Worker notification | Redis Pub/Sub | Avoids polling, low latency dispatch |
| Concurrency primitive | Mutex for queue, channels for dispatch | Mutex = shared state protection, channels = coordination |
| Persistence | Pluggable (memory/Redis) | Lets you demo both, shows abstraction thinking |
| Retry strategy | Exponential backoff | Avoids thundering herd on downstream failures |


## Future Improvements:
- Multiple QUEUE support
- A way to add handlers for different job types
- redis backend for production use, in-memory for development/testing
- metrics for monitoring queue length, job processing times, failure rates
