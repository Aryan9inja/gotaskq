package taskq

import (
	"context"

	internaljob "github.com/Aryan9inja/gotaskq/internal/job"
)

// Handler processes jobs of registered type
type Handler interface{
	// Handle runs the job and returns an error when the job should be retried or failed
	Handle(ctx context.Context, job *Job) error
}

// HandlerFunc adapts a function into a Handler
type HandlerFunc func(ctx context.Context, job *Job) error

// Handle runs f(ctx, job)
func (f HandlerFunc) Handle(ctx context.Context, job *Job) error{
	return f(ctx, job)
}

type handlerAdapter struct{
	handler Handler
}

func(a handlerAdapter) Handle(ctx context.Context, job *internaljob.Job) error{
	return a.handler.Handle(ctx, wrapJob(job))
}