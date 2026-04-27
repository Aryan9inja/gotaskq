package dlq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/metrics"
	"github.com/redis/go-redis/v9"
)

const redisDlqKeyPrefix = "gotaskq:dlq"
const defaultDlqLabel = "dlq"

var (
	ErrRedisClientNil = errors.New("redis client is nil")
	ErrJobNil         = errors.New("job is nil")
	ErrEmptyJobID     = errors.New("job id is empty")
	ErrJobNotDead     = errors.New("job status is not DEAD")
	ErrJobNotFound    = errors.New("dead job not found")
	ErrInvalidLimit   = errors.New("limit must be greater than 0")
)

type DlqInterface interface {
	Save(ctx context.Context, j *job.Job) error
	Get(ctx context.Context, id string) (*job.Job, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, limit int64) ([]*job.Job, error)
}

type RedisDlq struct {
	client     redis.UniversalClient
	dlqKey     string
	payloadKey string
}

func NewRedisDlq(redisClient redis.UniversalClient) (*RedisDlq, error) {
	if redisClient == nil {
		return nil, ErrRedisClientNil
	}

	return &RedisDlq{
		client:     redisClient,
		dlqKey:     fmt.Sprintf("%s:dead", redisDlqKeyPrefix),
		payloadKey: fmt.Sprintf("%s:payloads", redisDlqKeyPrefix),
	}, nil
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

// ==================
// DLQ implementation
// ==================
func (dlq *RedisDlq) Save(ctx context.Context, j *job.Job) error {
	if err := validateContext(ctx); err != nil {
		return err
	}

	if j == nil {
		return ErrJobNil
	}

	if j.ID == "" {
		return ErrEmptyJobID
	}

	if j.Status != job.StatusDead {
		return ErrJobNotDead
	}

	payload, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("marshal dead job %s failed: %w", j.ID, err)
	}

	_, err = dlq.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, dlq.payloadKey, j.ID, payload)
		pipe.ZAdd(ctx, dlq.dlqKey, redis.Z{
			Score:  float64(time.Now().UnixMilli()),
			Member: j.ID,
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("save dead job %s in redis failed: %w", j.ID, err)
	}

	metrics.IncJobsDead(defaultDlqLabel, j.Type)

	return nil
}

func (dlq *RedisDlq) Get(ctx context.Context, id string) (*job.Job, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}

	if id == "" {
		return nil, ErrEmptyJobID
	}

	raw, err := dlq.client.HGet(ctx, dlq.payloadKey, id).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrJobNotFound
		}
		return nil, fmt.Errorf("get dead job %s in redis failed: %w", id, err)
	}

	var output job.Job
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return nil, fmt.Errorf("unmarshal dead job %s failed: %w", id, err)
	}

	return &output, nil
}

func (dlq *RedisDlq) Delete(ctx context.Context, id string) error {
	if err := validateContext(ctx); err != nil {
		return err
	}

	if id == "" {
		return ErrEmptyJobID
	}

	_, err := dlq.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HDel(ctx, dlq.payloadKey, id)
		pipe.ZRem(ctx, dlq.dlqKey, id)
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete dead job %s in redis failed: %w", id, err)
	}

	return nil
}

func (dlq *RedisDlq) List(ctx context.Context, limit int64) ([]*job.Job, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}

	if limit <= 0 {
		return nil, ErrInvalidLimit
	}

	ids, err := dlq.client.ZRevRange(ctx, dlq.dlqKey, 0, limit-1).Result()
	if err != nil {
		return nil, fmt.Errorf("list dead jobs from redis dlq failed: %w", err)
	}

	if len(ids) == 0 {
		return []*job.Job{}, nil
	}

	fields := make([]string, len(ids))
	copy(fields, ids)

	rawJobs, err := dlq.client.HMGet(ctx, dlq.payloadKey, fields...).Result()
	if err != nil {
		return nil, fmt.Errorf("list dead jobs payload failed: %w", err)
	}

	output := make([]*job.Job, 0, len(rawJobs))
	for i, raw := range rawJobs {
		if raw == nil {
			continue
		}

		payload, err := redisStringValue(raw)
		if err != nil {
			return nil, fmt.Errorf("decode dead job payload for id %s failed: %w", ids[i], err)
		}

		var deadJob job.Job
		if err := json.Unmarshal([]byte(payload), &deadJob); err != nil {
			return nil, fmt.Errorf("unmarshal dead job %s failed: %w", ids[i], err)
		}

		output = append(output, &deadJob)
	}

	return output, nil
}
