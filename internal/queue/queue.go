package queue

import (
	"context"

	"github.com/Aryan9inja/gotaskq/internal/job"
)

type Queue interface {
	Enqueue(ctx context.Context, job *job.Job) error
	Dequeue(ctx context.Context) (job *job.Job, error error)
	Len() int
	Name() string
}

type Notifications interface{
	SubscribeNotifications(ctx context.Context) (<-chan struct{}, func(), error)
}