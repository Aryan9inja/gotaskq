package queue

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/job"
)

// ===================
// Heap Implementation
// ===================
type jobHeap []*job.Job

func (jobH jobHeap) Len() int { return len(jobH) }

// How heap will sort
func (jobH jobHeap) Less(i, j int) bool {
	// 1. Sorting based on runAfter
	if !jobH[i].RunAfter.Equal(jobH[j].RunAfter) {
		return jobH[i].RunAfter.Before(jobH[j].RunAfter)
	}

	// 2. Sort based on priority
	if jobH[i].Priority != jobH[j].Priority {
		return jobH[i].Priority > jobH[j].Priority
	}

	// 3. Sort based on created at
	if !jobH[i].CreatedAt.Equal(jobH[j].CreatedAt) {
		return jobH[i].CreatedAt.Before(jobH[j].CreatedAt)
	}

	return jobH[i].ID < jobH[j].ID
}

func (jobH jobHeap) Swap(i, j int) {
	jobH[i], jobH[j] = jobH[j], jobH[i]
}

func (jobH *jobHeap) Push(x any) {
	*jobH = append(*jobH, x.(*job.Job))
}

func (jobH *jobHeap) Pop() any {
	old := *jobH
	n := len(old)
	item := old[n-1]
	*jobH = old[:n-1]
	return item
}

// ===========================
// Memory Queue Implementation
// ===========================
type MemoryQueue struct {
	name string
	jobH jobHeap
	mu   sync.RWMutex
}

func NewMemoryQueue(name string) *MemoryQueue {
	memQueue := &MemoryQueue{
		name: name,
		jobH: make(jobHeap, 0),
	}

	heap.Init(&memQueue.jobH)
	return memQueue
}

func (memQueue *MemoryQueue) Enequeue(ctx context.Context, job *job.Job) error {
	memQueue.mu.Lock()
	defer memQueue.mu.Unlock()

	// Check if runAfter is not set
	if job.RunAfter.IsZero() {
		job.RunAfter = job.CreatedAt.Add(job.Delay)
	}

	if job.RunAfter.Before(time.Now()) {
		return errors.New("job should run now or after now")
	}

	heap.Push(&memQueue.jobH, job)
	return nil
}

func (memQueue *MemoryQueue) Dequeue(ctx context.Context) (j *job.Job, error error) {
	memQueue.mu.Lock()
	defer memQueue.mu.Unlock()

	if memQueue.jobH.Len() == 0 {
		return nil, errors.New("queue is empty")
	}

	// Peek at top element
	top := memQueue.jobH[0]

	if top.RunAfter.After(time.Now()) {
		return nil, errors.New("no jobs ready")
	}

	j = heap.Pop(&memQueue.jobH).(*job.Job)
	return j, nil
}

func (memQueue *MemoryQueue) Len() int{
	memQueue.mu.RLock()
	defer memQueue.mu.RUnlock()
	return memQueue.jobH.Len()
}

func (memQueue *MemoryQueue) Name() string{
	return memQueue.name
}
