package queue

import (
	"strings"

	"github.com/redis/go-redis/v9"
)

type Factory interface {
	New(name string) (Queue, error)
}

type MemoryFactory struct{}

func NewMemoryFactory() *MemoryFactory {
	return &MemoryFactory{}
}

func (f *MemoryFactory) New(name string) (Queue, error) {
	queueName := strings.TrimSpace(name)
	if queueName == "" {
		return nil, ErrEmptyQueueName
	}

	return NewMemoryQueue(queueName), nil
}

type RedisFactory struct {
	client redis.UniversalClient
}

func NewRedisFactory(client redis.UniversalClient) (*RedisFactory, error) {
	if client == nil {
		return nil, ErrRedisClientNil
	}

	return &RedisFactory{client: client}, nil
}

func (f *RedisFactory) New(name string) (Queue, error) {
	return NewRedisQueue(name, f.client)
}
