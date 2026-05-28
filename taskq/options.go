package taskq

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultAddr          = ":8000"
	defaultMaxRetryDelay = 5 * time.Second
	defaultNumWorkers    = 1
	defaultTTL           = 10
)

// Options configure taskq behaviour
type Options struct {
	// Addr is the HTTP address the server listen on
	// If Addr is empty, New uses ":8000"
	Addr string

	// MaxRetryDelay caps the retry backoff delay for a job
	// If MaxRetryDelay is zero or negative, New uses 5 seconds
	MaxRetryDelay time.Duration

	// NumWorkers is the number of background workers that process jobs
	// If NumWorkers is zero or negative, New uses 1 worker
	NumWorkers int

	// UseRedis enables redis based queue and job storage
	// Redis is also enabled automatically, when RedisClient is set
	UseRedis bool

	// RedisUrl is parsed using redis.parseUrl if useRedis is true and RedisClient is nil
	RedisUrl string

	// RedisClient is an optional Redis client supplied by the caller
	// When RedisClient is set, taskq will use it, but will not close it on Stop
	RedisClient redis.UniversalClient

	// TTL is the time to live for job in minutes
	// After TTL is reached job is deleted from storage
	// When TTL is zero or negative it will use 10 as value (10 minutes)
	TTL int

	// DefaultQueueName is the name of default queue created at server start
	// If DefaultQueueName is empty, it will be named "default"
	DefaultQueueName string
}

func normalizeOptions(opts Options) Options {
	if opts.Addr == "" {
		opts.Addr = defaultAddr
	}

	if opts.MaxRetryDelay <= 0 {
		opts.MaxRetryDelay = 5 * time.Second
	}

	if opts.NumWorkers <= 0 {
		opts.NumWorkers = 1
	}

	if opts.TTL <= 0 {
		opts.TTL = 10
	}

	if opts.DefaultQueueName == ""{
		opts.DefaultQueueName = "default"
	}

	return opts
}

func redisClientFromOptions(opts Options) (redis.UniversalClient, bool, error) {
	if opts.RedisClient != nil {
		if err := opts.RedisClient.Ping(context.Background()).Err(); err != nil {
			return nil, false, fmt.Errorf("ping redis: %w", err)
		}
		return opts.RedisClient, false, nil
	}

	if strings.TrimSpace(opts.RedisUrl) == "" {
		return nil, false, errors.New("taskq : redis url is required when use redis is true")
	}

	redisOpts, err := redis.ParseURL(opts.RedisUrl)
	if err != nil {
		return nil, false, fmt.Errorf("parse redis url : %w", err)
	}

	client := redis.NewClient(redisOpts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, false, fmt.Errorf("ping redis: %w", err)
	}

	return client, true, nil
}
