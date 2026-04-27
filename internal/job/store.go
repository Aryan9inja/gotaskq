package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisJobKeyPrefix = "gotaskq:job"

type Store interface {
	Save(ctx context.Context, job *Job) error
	Get(ctx context.Context, id string) (*Job, error)
	UpdateStatus(ctx context.Context, id string, status Status) error
	Delete(ctx context.Context, id string) error
}

var (
	ErrRedisClientNil = errors.New("redis client is nil")
	ErrJobNil         = errors.New("job is nil")
	ErrEmptyJobID     = errors.New("job id is empty")
	ErrJobNotFound    = errors.New("job not found")
)

// ====================================
// Struct defination and initialization
// ====================================
type inMemoryStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

type redisStore struct {
	client redis.UniversalClient
}

func NewMemoryStore() *inMemoryStore {
	return &inMemoryStore{
		jobs: make(map[string]*Job),
	}
}

func NewRedisStore(redisClient redis.UniversalClient) (*redisStore, error) {
	if redisClient == nil {
		return nil, ErrRedisClientNil
	}

	return &redisStore{client: redisClient}, nil
}

// ========================
// General Helper Functions
// ========================
func validateContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("Context is null")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return nil
}

// ==========================
// In Memory Helper Functions
// ==========================
func isValidTransition(from, to Status) bool {
	switch from {
	case StatusPending:
		return to == StatusRunning
	case StatusRunning:
		return to == StatusDone || to == StatusFailed
	case StatusFailed:
		return to == StatusPending || to == StatusDead
	default:
		return false
	}
}

// ==============================
// In Memory Store Implementation
// ==============================
func (store *inMemoryStore) Save(ctx context.Context, job *Job) error {
	err := validateContext(ctx)
	if err != nil {
		return err
	}

	if job == nil {
		return ErrJobNil
	}

	if job.ID == "" {
		return ErrEmptyJobID
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	store.jobs[job.ID] = job

	return nil
}

func (store *inMemoryStore) Get(ctx context.Context, id string) (*Job, error) {
	err := validateContext(ctx)
	if err != nil {
		return nil, err
	}

	if id == "" {
		return nil, ErrEmptyJobID
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	job, exists := store.jobs[id]
	if !exists {
		return nil, ErrJobNotFound
	}

	return job, nil
}

func (store *inMemoryStore) UpdateStatus(ctx context.Context, id string, status Status) error {
	err := validateContext(ctx)
	if err != nil {
		return err
	}

	if id == "" {
		return ErrEmptyJobID
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	job, exists := store.jobs[id]
	if !exists {
		return ErrJobNotFound
	}

	if !isValidTransition(job.Status, status) {
		return fmt.Errorf("invalid transition: %s -> %s", string(job.Status), string(status))
	}

	job.Status = status
	job.UpdatedAt = time.Now()

	return nil
}

func (store *inMemoryStore) Delete(ctx context.Context, id string) error {
	err := validateContext(ctx)
	if err != nil {
		return err
	}

	if id == "" {
		return ErrEmptyJobID
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	delete(store.jobs, id)

	return nil
}

// ============================
// Redis based Helper Functions
// ============================
func encodeDurationValue(v time.Duration) string {
	return strconv.FormatInt(int64(v), 10)
}

func decodeDurationValue(v string) (time.Duration, error) {
	if v == "" {
		return 0, nil
	}

	parsed, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, err
	}

	return time.Duration(parsed), nil
}

func encodeTimeValue(v time.Time) string {
	if v.IsZero() {
		return ""
	}

	return v.Format(time.RFC3339Nano)
}

func decodeTimeValue(v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}

	parsed, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}, err
	}

	return parsed, nil
}

func jobToRedisHash(j *Job) (map[string]any, error) {
	if j == nil {
		return nil, ErrJobNil
	}

	if j.ID == "" {
		return nil, ErrEmptyJobID
	}

	payload, err := json.Marshal(j.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload for job %s: %w", j.ID, err)
	}

	return map[string]any{
		"id":          j.ID,
		"type":        j.Type,
		"payload":     string(payload),
		"status":      string(j.Status),
		"priority":    strconv.Itoa(j.Priority),
		"delay":       encodeDurationValue(j.Delay),
		"max_retries": strconv.Itoa(j.MaxRetries),
		"retry_count": strconv.Itoa(j.RetryCount),
		"error":       j.Error,
		"created_at":  encodeTimeValue(j.CreatedAt),
		"updated_at":  encodeTimeValue(j.UpdatedAt),
		"run_after":   encodeTimeValue(j.RunAfter),
	}, nil
}

func redisHashToJob(values map[string]string) (*Job, error) {
	if len(values) == 0 {
		return nil, ErrJobNotFound
	}

	priority, err := strconv.Atoi(values["priority"])
	if err != nil {
		return nil, fmt.Errorf("parse priority: %w", err)
	}

	maxRetries, err := strconv.Atoi(values["max_retries"])
	if err != nil {
		return nil, fmt.Errorf("parse max_retries: %w", err)
	}

	retryCount, err := strconv.Atoi(values["retry_count"])
	if err != nil {
		return nil, fmt.Errorf("parse retry_count: %w", err)
	}

	delay, err := decodeDurationValue(values["delay"])
	if err != nil {
		return nil, fmt.Errorf("parse delay: %w", err)
	}

	createdAt, err := decodeTimeValue(values["created_at"])
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	updatedAt, err := decodeTimeValue(values["updated_at"])
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	runAfter, err := decodeTimeValue(values["run_after"])
	if err != nil {
		return nil, fmt.Errorf("parse run_after: %w", err)
	}

	var payload json.RawMessage
	if rawPayload := values["payload"]; rawPayload == "" {
		payload = json.RawMessage(rawPayload)
	}

	return &Job{
		ID:         values["id"],
		Type:       values["type"],
		Payload:    payload,
		Status:     Status(values["status"]),
		Priority:   priority,
		Delay:      delay,
		MaxRetries: maxRetries,
		RetryCount: retryCount,
		Error:      values["error"],
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
		RunAfter:   runAfter,
	}, nil
}

func redisJobKey(id string) string {
	return fmt.Sprintf("%s:%s", redisJobKeyPrefix, id)
}

func redisInterfaceToInt64(v any) (int64, error) {
	switch val := v.(type) {
	case int64:
		return val, nil
	case int:
		return int64(val), nil
	case string:
		parsed, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}

func redisStringValue(v any) (string, error) {
	switch val := v.(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	default:
		return "", fmt.Errorf("unsupported type %T", v)
	}
}

// ==================================
// Lua Script to update status of job
// ==================================
/**
* Takes one key - RedisJobKey
* Take 2 arguments - to status, currTime
* Return Format ->
* {0} = job is invalid
* {1, string} = you tried wrong state transition and here is the state transition
* {2} = everything succeeded
 */
const updateStatusLua = `
local currentStatus = redis.call('HGET', KEYS[1], 'status')

if not currentStatus then
	return {0}
end

local from = currentStatus
local to = ARGV[1]
local now = ARGV[2]

local valid = false

if from == 'PENDING' then
	valid = (to == 'RUNNING')
elseif from == 'RUNNING' then
	valid = (to == 'DONE' or to == 'FAILED')
elseif from == 'FAILED' then
	valid = (to == 'PENDING' or to == 'DEAD')
end

if not valid then 
	return {1, from}
end

redis.call('HSET', KEYS[1], 'status', to, 'updated_at', now)
return {2}
`

// ================================
// Redis based Store Implementation
// ================================
func (store *redisStore) Save(ctx context.Context, job *Job) error {
	err := validateContext(ctx)
	if err != nil {
		return err
	}

	if job == nil {
		return ErrJobNil
	}

	if job.ID == "" {
		return ErrEmptyJobID
	}

	fields, err := jobToRedisHash(job)
	if err != nil {
		return err
	}

	if _, err := store.client.HSet(ctx, redisJobKey(job.ID), fields).Result(); err != nil {
		return fmt.Errorf("save job %s to redis failed: %w", job.ID, err)
	}

	return nil
}

func (store *redisStore) Get(ctx context.Context, id string) (*Job, error) {
	err := validateContext(ctx)
	if err != nil {
		return nil, err
	}

	if id == "" {
		return nil, ErrEmptyJobID
	}

	values, err := store.client.HGetAll(ctx, redisJobKey(id)).Result()
	if err != nil {
		return nil, fmt.Errorf("get job %s from redis failed: %w", id, err)
	}

	job, err := redisHashToJob(values)
	if err != nil {
		return nil, err
	}

	return job, nil
}

func (store *redisStore) Delete(ctx context.Context, id string) error {
	err := validateContext(ctx)
	if err != nil {
		return err
	}

	if id == "" {
		return ErrEmptyJobID
	}

	if _, err := store.client.Del(ctx, redisJobKey(id)).Result(); err != nil {
		return fmt.Errorf("delete job %s from redis failed: %w", id, err)
	}

	return nil
}

func (store *redisStore) UpdateStatus(ctx context.Context, id string, status Status) error {
	err := validateContext(ctx)
	if err != nil {
		return err
	}

	if id == "" {
		return ErrEmptyJobID
	}

	res, err := store.client.Eval(
		ctx,
		updateStatusLua,
		[]string{redisJobKey(id)},
		string(status),
		encodeTimeValue(time.Now()),
	).Result()
	if err != nil {
		return fmt.Errorf("update job %s status in redis failed: %w", id, err)
	}

	parts, ok := res.([]any)
	if !ok || len(parts) == 0 {
		return errors.New("unexpected redis status update response")
	}

	code, convErr := redisInterfaceToInt64(parts[0])
	if convErr != nil {
		return fmt.Errorf("invalid redis status update code: %w", err)
	}

	switch code {
	case 0:
		return ErrJobNotFound
	case 1:
		current := ""
		if len(parts) > 1 {
			current, _ = redisStringValue(parts[1])
		}
		if current == "" {
			current = "unknown"
		}
		return fmt.Errorf("invalid transition: %s -> %s", current, string(status))
	case 2:
		return nil
	default:
		return fmt.Errorf("unknown redis status update code: %d", code)
	}
}
