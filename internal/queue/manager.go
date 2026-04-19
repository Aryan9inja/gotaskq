package queue

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

var (
	ErrNilQueue           = errors.New("queue is nil")
	ErrEmptyQueueName     = errors.New("queue name cannot be empty")
	ErrQueueAlreadyExists = errors.New("queue already exists")
	ErrQueueNotFound      = errors.New("queue not found")
	ErrDefaultQueueNotSet = errors.New("default queue is not set")
)

type QueueManager struct {
	queues           map[string]Queue
	defaultQueueName string
	mu               sync.RWMutex
}

func NewQueueManager() *QueueManager {
	return &QueueManager{
		queues: make(map[string]Queue),
	}
}

func (queueMan *QueueManager) Register(q Queue) error {
	if q == nil {
		return ErrNilQueue
	}

	name := strings.TrimSpace(q.Name())
	if name == "" {
		return ErrEmptyQueueName
	}

	queueMan.mu.Lock()
	defer queueMan.mu.Unlock()

	if _, exists := queueMan.queues[name]; exists {
		return ErrQueueAlreadyExists
	}

	queueMan.queues[name] = q

	if queueMan.defaultQueueName == "" {
		queueMan.defaultQueueName = name
	}

	return nil
}

func (queueMan *QueueManager) Get(name string) (Queue, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrEmptyQueueName
	}

	queueMan.mu.RLock()
	defer queueMan.mu.RUnlock()

	q, exists := queueMan.queues[name]
	if !exists {
		return nil, ErrQueueNotFound
	}

	return q, nil
}

func (queueMan *QueueManager) SetDefault(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrEmptyQueueName
	}

	queueMan.mu.Lock()
	defer queueMan.mu.Unlock()

	if _, exists := queueMan.queues[name]; !exists {
		return ErrQueueNotFound
	}

	queueMan.defaultQueueName = name

	return nil
}

func (queueMan *QueueManager) DefaultQueue() (Queue, error) {
	queueMan.mu.RLock()
	defer queueMan.mu.RUnlock()

	if queueMan.defaultQueueName == "" {
		return nil, ErrDefaultQueueNotSet
	}

	q, exists := queueMan.queues[queueMan.defaultQueueName]
	if !exists {
		return nil, ErrQueueNotFound
	}

	return q, nil
}

func (queueMan *QueueManager) DefaultName() string {
	queueMan.mu.RLock()
	defer queueMan.mu.RUnlock()

	return queueMan.defaultQueueName
}

func (queueMan *QueueManager) ListNames() []string {
	queueMan.mu.RLock()
	defer queueMan.mu.RUnlock()

	names := make([]string, 0, len(queueMan.queues))
	for name := range queueMan.queues {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
