# GoTaskQ
GoTaskQ is a Go-based job queue with HTTP ingestion, worker pool execution, retries, DLQ handling, and Prometheus metrics. It runs in-memory by default and can switch to Redis for durability.

I Streamed the development of GoTaskQ live on YouTube, and the full playlist is available for viewing. The project is structured to allow incremental development and iterative releases, with a clear roadmap for future enhancements.
### Stream playlist: https://youtube.com/playlist?list=PLeNxLCYqfeHTxEQ33yuftg5F-SzKoZKPh&si=GhwWzBpnHBsYaaJ3

## Architecture Overview
### HLD
<img width="1314" height="792" alt="HLD_gotaskq" src="https://github.com/user-attachments/assets/65e73fd4-00a6-48e5-8b6f-15a730f9504b" />

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
   - Worker pool dequeues ready jobs, marks `RUNNING`, executes the handler, then marks `DONE` or `FAILED`.
   - Planned: when multiple queues are registered, workers will use a round-robin snapshot to choose which queue to poll first.
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
- `NUM_WORKERS` (default: `10`)
- `MAX_DELAY` (default: `5000`) caps retry backoff
- `MAX_RETRIES` (reserved; per-job `max_retries` is used today)
- `BASE_DELAY` (reserved; not wired yet)
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

## Known Issues and Proposed Fixes (as of 2026-05-06)
1. Dynamic queue processing
   - Issue: Queues created via `POST /queue/{name}` are registered in the manager but not added to the worker pool, so non-default queues may not be processed.
   - Proposed fix: Inject a queue registrar into API handlers and call `AddQueue()` after successful registration.
2. Retry engine queue binding
   - Issue: `RetryEngine` stores a single queue, so retries from non-default queues may re-enqueue to the wrong queue.
   - Proposed fix: Pass the current queue into `HandleFailure()` and avoid storing a queue in the engine.
3. Worker pool wiring
   - Issue: `cmd/server` wiring still assumes a single-queue pool and does not register the default queue via the pool API.
   - Proposed fix: Update construction so the default queue is added through `AddQueue()`.
4. Completed-job retention
   - Issue: Jobs are deleted from the store immediately after success, which makes post-completion queries hard.
   - Proposed fix: Add a short retention window (e.g., `COMPLETE_JOB_TTL`) before cleanup.

## Benchmark
Command:
```
hey -n 100000 -c 500 -m POST \
  -H "Content-Type: application/json" \
  -d '{"type":"logger","payload":{"msg":"stress"},"priority":5}' \
  http://localhost:8000/jobs
```

Summary:
- Total: 11.4043 secs
- Slowest: 0.1063 secs
- Fastest: 0.0169 secs
- Average: 0.0569 secs
- Requests/sec: 8768.6479
- Total data: 29766621 bytes
- Size/request: 297 bytes

Latency distribution:
- 10% in 0.0532 secs
- 25% in 0.0543 secs
- 50% in 0.0558 secs
- 75% in 0.0578 secs
- 90% in 0.0607 secs
- 95% in 0.0670 secs
- 99% in 0.0766 secs

Status codes:
- 201: 100000 responses

Metrics snapshot:
- `gotaskq_jobs_enqueued_total{queue="default",type="logger"}`: 100000
- `gotaskq_jobs_processed_total{queue="default",status="done",type="logger"}`: 100000
- `gotaskq_active_workers`: 100

## Future Scope
1. Introduce more HTTP routes.
2. Allow users to add custom handlers.
3. Add a metrics UI (Grafana or Prometheus UI).
4. Make metrics code production-ready and allow a user-chosen registry.
