#!/usr/bin/env bash
set -euo pipefail

if ! command -v hey >/dev/null 2>&1; then
  echo "hey is required. Install from https://github.com/rakyll/hey" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required." >&2
  exit 1
fi

BASE_URL="${BASE_URL:-http://localhost:8000}"
QUEUE_COUNT="${QUEUE_COUNT:-3}"
REQUESTS="${REQUESTS:-50000}"
CONCURRENCY="${CONCURRENCY:-200}"
QUEUE_PREFIX="${QUEUE_PREFIX:-stress}"
QUEUE_SUFFIX="${QUEUE_SUFFIX:-}"
JOB_TYPE="${JOB_TYPE:-logger}"
PAYLOAD="${PAYLOAD:-{\"msg\":\"stress\"}}"
PRIORITY="${PRIORITY:-5}"
MAX_RETRIES="${MAX_RETRIES:-0}"
CHECK_METRICS="${CHECK_METRICS:-1}"

if ! [[ "$QUEUE_COUNT" =~ ^[0-9]+$ ]] || [[ "$QUEUE_COUNT" -lt 1 ]]; then
  echo "QUEUE_COUNT must be a positive integer" >&2
  exit 1
fi

suffix=""
if [[ -n "$QUEUE_SUFFIX" ]]; then
  suffix="-$QUEUE_SUFFIX"
fi

queues=()
for ((i = 1; i <= QUEUE_COUNT; i++)); do
  queues+=("${QUEUE_PREFIX}-${i}${suffix}")
done

existing_queues="$(curl -s "${BASE_URL}/queue" || true)"

for q in "${queues[@]}"; do
  if echo "$existing_queues" | grep -q "\"${q}\""; then
    echo "queue ${q} already exists, reusing" >&2
    continue
  fi

  status="$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/queue/${q}")"
  if [[ "$status" != "201" ]]; then
    echo "failed to create queue ${q} (status ${status})" >&2
    exit 1
  fi

done

pids=()
for q in "${queues[@]}"; do
  hey -n "$REQUESTS" -c "$CONCURRENCY" -m POST \
    -H "Content-Type: application/json" \
    -d "{\"type\":\"${JOB_TYPE}\",\"payload\":${PAYLOAD},\"priority\":${PRIORITY},\"max_retries\":${MAX_RETRIES}}" \
    "${BASE_URL}/${q}/jobs" &
  pids+=("$!")
done

failed=0
for pid in "${pids[@]}"; do
  if ! wait "$pid"; then
    failed=1
  fi
done

if [[ "$failed" -ne 0 ]]; then
  echo "one or more hey runs failed" >&2
  exit 1
fi

if [[ "$CHECK_METRICS" == "1" ]]; then
  echo "waiting 20 seconds for jobs to finish processing..." >&2
  sleep 20
  echo "metrics snapshot (enqueued/processed per queue):" >&2
  metrics="$(curl -s "${BASE_URL}/metrics" || true)"
  for q in "${queues[@]}"; do
    echo "$metrics" | grep -F "queue=\"${q}\"" | grep -E "gotaskq_jobs_(enqueued|processed)_total" || true
  done
fi
