package queue

import (
	"errors"
	"sync"
)

type QueueManager struct {
	queues map[string]Queue
	mu     sync.RWMutex
}

func NewQueueManager() *QueueManager {
	return &QueueManager{
		queues: make(map[string]Queue),
	}
}

func (queueMan *QueueManager) Register(q Queue) error {
	queueMan.mu.Lock()
	defer queueMan.mu.Unlock()

	if _, exists := queueMan.queues[q.Name()]; exists {
		return errors.New("queue already exists")
	}

	queueMan.queues[q.Name()] = q
	return nil
}

func (queueMan *QueueManager) Get(name string) (Queue,error){
	queueMan.mu.RLock()
	defer queueMan.mu.RUnlock()

	q, exists := queueMan.queues[name]
	if !exists{
		return nil, errors.New("queue not found")
	}

	return q, nil
}
