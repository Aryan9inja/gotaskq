package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/redis/go-redis/v9"
)

const redisQueueKeyPrefix = "gotaskq:queue"
const redisQueuePayloadKeySuffix = "payloads"
const redisSortableFieldWidth = 20

var redisSortableSignBit = uint64(1) << 63

var (
	ErrRedisClientNil = errors.New("redis client is nil")
)

type RedisQueue struct {
	name       string
	key        string
	payloadKey string
	client     redis.UniversalClient
}

// ================
// Helper Functions
// ================
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

func encodeAscendingInt64(v int64) string {
	return fmt.Sprintf("%0*d", redisSortableFieldWidth, uint64(v)^redisSortableSignBit)
}

func encodeDescendingInt64(v int64) string {
	return fmt.Sprintf("%0*d", redisSortableFieldWidth, ^(uint64(v) ^ redisSortableSignBit))
}

func (redisQueue *RedisQueue) memberForJob(j *job.Job) string {
	return fmt.Sprintf(
		"%s|%s|%s",
		encodeDescendingInt64(int64(j.Priority)),
		encodeAscendingInt64(int64(j.CreatedAt.UnixNano())),
		j.ID,
	)
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

// ================================
// Lua Script for dequeue operation
// ================================

// dequeueReadyLua -> atomically return one ready job and remove it from the queue
// Return shape:
// [0]                            -> queue empty
// [1]                            -> no jobs ready
// [2, <job_json_string>]         -> success
const dequeueReadyLua = `
local nowMs = tonumber(ARGV[1])

while true do
	local items = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
	if #items == 0 then
		return {0}
	end

	local member = items[1]
	local score = tonumber(items[2])

	if score > nowMs then
		return {1}
	end

	local payload = redis.call('HGET', KEYS[2], member)
	if payload then
		redis.call('ZREM', KEYS[1], member)
		redis.call('HDEL', KEYS[2], member)
		return {2, payload}
	end

	redis.call('ZREM', KEYS[1], member)
end
`

// ==============
// Main Functions
// ==============
func NewRedisQueue(name string, client redis.UniversalClient) (*RedisQueue, error) {
	queueName := strings.TrimSpace(name)
	if queueName == "" {
		return nil, ErrEmptyQueueName
	}

	if client == nil {
		return nil, ErrRedisClientNil
	}

	return &RedisQueue{
		name:       queueName,
		key:        fmt.Sprintf("%s:%s:pending", redisQueueKeyPrefix, queueName),
		payloadKey: fmt.Sprintf("%s:%s:%s", redisQueueKeyPrefix, queueName, redisQueuePayloadKeySuffix),
		client:     client,
	}, nil
}

func (redisQueue *RedisQueue) Enqueue(ctx context.Context, j *job.Job) error {
	if err := validateContext(ctx); err != nil {
		return err
	}

	if j == nil {
		return errors.New("job is nil")
	}

	now := time.Now()

	if j.RunAfter.IsZero() {
		j.RunAfter = j.CreatedAt.Add(j.Delay)
	}
	if j.RunAfter.Before(now) {
		j.RunAfter = now
	}

	member := redisQueue.memberForJob(j)
	payload, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("marshal job for queue: %w", err)
	}

	_, err = redisQueue.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, redisQueue.payloadKey, member, payload)
		pipe.ZAdd(ctx, redisQueue.key, redis.Z{
			Score:  float64(j.RunAfter.UnixMilli()),
			Member: member,
		})
		return nil
	})

	if err != nil {
		return fmt.Errorf("enqueue redis transaction failed: %w", err)
	}

	return nil
}

func (redisQueue *RedisQueue) Dequeue(ctx context.Context) (j *job.Job, error error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}

	res, err := redisQueue.client.Eval(
		ctx,
		dequeueReadyLua,
		[]string{redisQueue.key, redisQueue.payloadKey},
		time.Now().UnixMilli(),
	).Result()
	if err != nil {
		return nil, fmt.Errorf("dequeue redis eval failed: %w", err)
	}

	parts, ok := res.([]any)
	if !ok || len(parts) == 0 {
		return nil, errors.New("unexpected redis queue response")
	}

	code, convErr := redisInterfaceToInt64(parts[0])
	if convErr != nil {
		return nil, fmt.Errorf("invalid redis dequeue status code: %w", convErr)
	}

	switch code {
	case 0:
		return nil, errors.New("queue is empty")
	case 1:
		return nil, errors.New("no jobs ready")
	case 2:
		if len(parts) < 2 {
			return nil, errors.New("missing job payload type in redis dequeue response")
		}

		raw, err := redisStringValue(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid job payload type in redis dequeue response: %w", err)
		}

		var output job.Job
		if err := json.Unmarshal([]byte(raw), &output); err != nil {
			return nil, fmt.Errorf("unmarshal dequeue job failed: %w", err)
		}

		return &output, nil
	default:
		return nil, fmt.Errorf("unknown redis dequeue status code: %d", code)
	}
}

func (redisQueue *RedisQueue) Len() int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	count, err := redisQueue.client.ZCard(ctx, redisQueue.key).Result()
	if err != nil {
		return 0
	}
	return int(count)
}

func (redisQueue *RedisQueue) Name() string {
	return redisQueue.name
}
