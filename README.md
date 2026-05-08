# GoTaskQ
GoTaskQ is a Go-based job queue with HTTP ingestion, worker pool execution, retries, DLQ handling, and Prometheus metrics. It runs in-memory by default and can switch to Redis for durability.

I Streamed the development of GoTaskQ live on YouTube, and the full playlist is available for viewing. The project is structured to allow incremental development and iterative releases, with a clear roadmap for future enhancements.
### Stream playlist: https://youtube.com/playlist?list=PLeNxLCYqfeHTxEQ33yuftg5F-SzKoZKPh&si=GhwWzBpnHBsYaaJ3

## Architecture Overview
### HLD
<img width="1535" height="740" alt="HLD v1.1" src="https://github.com/user-attachments/assets/8756c984-ee9e-40ac-8cd7-df116027888d" />


### Architecture decisions
1. API ingress
   - `POST /jobs` validates input, assigns a Snowflake ID, and persists job state.
2. Persistence
   - In-memory store uses a mutex-protected map.
   - Redis store saves job hashes and enforces status transitions with Lua.
3. Scheduling
   - Memory queue uses a heap ordered by `run_after`, `priority`, `created_at`, then `id`.
   - Redis queue uses a sorted set plus a payload hash and publishes notifications.
4. Execution
   - Worker pool starts `NUM_WORKERS` total goroutines for the process.
   - Workers select across registered queues in round-robin order and process the first ready job they find.
   - Workers mark jobs `RUNNING`, execute the handler, then mark `DONE` or `FAILED`.
5. Retry and DLQ
   - Failed jobs are re-queued with exponential backoff until retries are exhausted.
   - Exhausted jobs are marked `DEAD` and stored in Redis DLQ (when enabled).
6. Metrics
   - Prometheus metrics capture enqueue/processed counts, active workers, queue depth, retries, and job durations.

## Components
- `cmd/server`: app wiring, handler registration, worker pool startup, and graceful shutdown.
- `config`: environment-driven configuration.
- `internal/api`: HTTP server and handlers (`/jobs`, `/dlq`, `/metrics`).
- `internal/job`: job model and store implementations (memory/Redis).
- `internal/queue`: queue interface, memory heap, Redis queue + Pub/Sub notifications.
- `internal/worker`: worker pool, job execution, status updates, metrics emission.
- `internal/handler`: handler registry for job types.
- `internal/retry`: retry engine with exponential backoff + jitter.
- `internal/dlq`: Redis-backed dead-letter store.
- `internal/metrics`: Prometheus metrics registration and helpers.
- `pkg/snowflake`: unique, time-ordered IDs.

## Worker Model
GoTaskQ uses a global worker pool with a queue selector. The default queue is registered at startup, and newly created queues are added to the selector only after queue registration succeeds.

`NUM_WORKERS` is the total process concurrency, not a per-queue value. For example, `NUM_WORKERS=10` creates 10 worker goroutines whether the process has one queue or five queues.

Each worker snapshots the current queue list, rotates the starting queue, and tries to dequeue a ready job. This keeps concurrency bounded while allowing dynamic queues. Retry handling still re-enqueues failed jobs onto the same queue that originally ran them.

The current selector is round-robin. Future queue policies can add weighted priority or strict priority without changing the global worker count.

## Setup
### Prerequisites
- Go 1.26.1+
- Redis (optional; required if `USE_REDIS=true`)

### Environment
Configuration is read from environment variables. A template is provided in `example.env`.

To load it in your shell:
```
cp example.env .env
set -a
source .env
set +a
```
Note: there is no built-in `.env` loader, so you must export variables before running.

### Run (in-memory)
```
export USE_REDIS=false
go run ./cmd/server
```

### Run (Redis)
```
export USE_REDIS=true
export REDIS_URL=redis://localhost:6379
go run ./cmd/server
```

### Redis (Docker, optional)
```
docker run --rm -p 6379:6379 redis:7
```

## Configuration
- `PORT` (default: `8000`)
- `NUM_WORKERS` (default: `10`) total worker goroutines for this process
- `MAX_DELAY` (default: `5000`) caps retry backoff
- `MAX_RETRIES` (reserved; per-job `max_retries` is used today)
- `BASE_DELAY` (reserved; not wired yet)
- `COMPLETE_JOB_TTL` (default: `5m`) keeps completed jobs queryable before cleanup
- `USE_REDIS` (`true` enables Redis backend)
- `REDIS_URL` (required when `USE_REDIS=true`)

## API
- `POST /jobs`
  - Request body:
    ```json
    {
      "type": "logger",
      "payload": {"msg": "hello"},
      "priority": 5,
      "delay": 0,
      "max_retries": 5
    }
    ```
   - `delay` is interpreted as seconds for the initial schedule.
- `GET /jobs/{id}`
- `GET /dlq` (Redis only)
- `POST /dlq/{id}/replay` (Redis only)
- `DELETE /dlq/{id}` (Redis only)
- `POST /queue/{name}` creates a queue and adds it to the worker selector after registration succeeds
- `GET /queue` lists registered queue names
- `GET /metrics` (Prometheus)

## Metrics
Prometheus metrics are exposed at `/metrics`. Key series:
- `gotaskq_jobs_enqueued_total`
- `gotaskq_jobs_processed_total`
- `gotaskq_job_duration_seconds`
- `gotaskq_queue_depth`
- `gotaskq_active_workers`
- `gotaskq_jobs_retried_total`
- `gotaskq_jobs_dead_total`

## Benchmark
**System Configuration:**
- **OS:** Linux x86_64
- **CPU:** AMD Ryzen 5 5600H with Radeon Graphics (6 Cores / 12 Threads)
- **RAM:** 16 GB

Command:
```
./scripts/stress_multi_queue.sh
```
This script creates 3 queues and sends 50,000 jobs to each queue concurrently using `hey` (150,000 total jobs).

### In-Memory Backend (Multi-Queue)
Summary (per queue):
- Total: ~1.95 secs
- Average Latency: ~0.007 secs
- Requests/sec: ~25,600 (Total ~76,800 req/sec across 3 queues)

Status codes:
- 201: 50,000 responses per queue

Metrics snapshot (after processing delay):
- `gotaskq_jobs_enqueued_total`: 50000 per queue
- `gotaskq_jobs_processed_total{status="done"}`: 50000 per queue

### Redis Backend (Multi-Queue)
Summary (per queue):
- Total: ~8.80 secs
- Average Latency: ~0.035 secs
- Requests/sec: ~5,680 (Total ~17,040 req/sec across 3 queues)

Status codes:
- 201: 50,000 responses per queue

Metrics snapshot (after processing delay):
- `gotaskq_jobs_enqueued_total`: 50000 per queue
- `gotaskq_jobs_processed_total{status="done"}`: 50000 per queue

### Performance Note
The **In-Memory** queue is significantly faster because operations are simple, thread-safe (mutex-protected) map and heap manipulations happening directly in the application's RAM without any I/O blocking.

The **Redis** queue is relatively slower (though still highly performant) due to:
1. **Network/Socket Overhead:** Every job enqueue and status update requires serializing data, sending it over a local socket, and waiting for the Redis response.
2. **Durability:** Redis manages persistence and executes complex data structure operations (Hashes, Sorted Sets) synchronously inside its single-threaded event loop.
3. **Round-Trips:** While the worker pool handles concurrent connections, each worker still pays the latency cost of multiple Redis commands (polling, popping, and updating job hashes).

## Future Scope
Convert this into a library with a clean API for embedding in other applications. The current server implementation will be refactored to use the library, and the library will be designed to allow users to build custom servers or integrate directly into their codebase without HTTP. The library will expose a clear API for job creation, queue management, and worker execution, while abstracting away the underlying storage and scheduling mechanics. This will enable greater flexibility and adoption in various Go applications.
