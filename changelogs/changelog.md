# Changelog

This file tracks release history and links to full version changelog documents.

## Version Index
- [v1.0-beta-redis-based-queue](v1.0-beta-redis-based-queue.md) - 2026-04-27
- [v1.0-alpha-in-memory-queue](v1.0-alpha-in-memory-queue.md) - 2026-04-17

## Version Notes

### v1.0-beta-redis-based-queue
- Full report: [v1.0-beta-redis-based-queue.md](v1.0-beta-redis-based-queue.md)
- Release scope: Redis-backed queue/store/DLQ execution path with in-memory fallback retained.
- Backend toggle: runtime backend selected via `USE_REDIS` and `REDIS_URL` with startup connectivity validation.
- Scheduling model: Redis sorted set + hash payload storage with atomic Lua dequeue honoring `run_after` readiness.
- Worker dispatch: Redis Pub/Sub queue notifications reduce idle polling latency for workers.
- Reliability path: retry handling is now wired into worker failure paths and dead jobs are persisted to DLQ after retry exhaustion.
- API expansion: DLQ listing, replay, and delete endpoints are available for operational recovery workflows.
- Current constraints: metrics remain uninstrumented, single default queue wiring, and DLQ endpoints require Redis-backed DLQ configuration.

### v1.0-alpha-in-memory-queue
- Full report: [v1.0-alpha-in-memory-queue.md](v1.0-alpha-in-memory-queue.md)
- Release scope: first complete in-memory execution path from HTTP submit to asynchronous worker completion.
- Runtime model: heap-based scheduler with deterministic dequeue order (`run_after` -> `priority` -> `created_at` -> `id`).
- Concurrency model: goroutine-based worker pool with context-driven stop and wait-group drain.
- Interface readiness: queue/store abstractions and handler lookup seam are in place for backend and execution-path evolution.
- Retry posture: exponential backoff, delay cap, and jitter are implemented; worker failure-path integration remains a pending step.
- Current constraints: in-memory state only, DLQ flow incomplete, metrics not yet instrumented, minimal API surface for alpha stabilization.
- Recommended use: local validation and integration prototyping before Redis-backed persistence and full reliability pathways.

