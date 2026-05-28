package taskq

import (
	"encoding/json"
	"time"

	internalJob "github.com/Aryan9inja/gotaskq/internal/job"
)

// Status defines current lifecycle state of a job
type Status string

const (
	// StatusPending means the job is waiting to be processed
	StatusPending Status = "PENDING"

	// StatusRunning means the job is currently being processed by a worker
	StatusRunning Status = "RUNNING"

	// StatusDone means the job has been processed successfully
	StatusDone    Status = "DONE"

	// StatusFailed means the job has failed to process with an error
	StatusFailed  Status = "FAILED"

	// StatusDead means the job has failed and exhausted all retry attempts
	StatusDead    Status = "DEAD"
)

type Job struct {
	// ID is a unique identifier for the job
	ID         string			`json:"id"`

	// Type is a string that identifies the type of the job, used to route to the correct handler
	Type       string			`json:"type"`

	// Payload is the JSON encoded data for the job, which will be passed to the handler
	Payload    json.RawMessage	`json:"payload"`

	// Status represents the current lifecycle state of the job
	Status     Status			`json:"status"`

	// Priority determines the order of job processing, higher priority jobs are processed first
	Priority   int				`json:"priority"`

	// Delay is the duration to wait before the job becomes eligible for processing
	Delay      time.Duration	`json:"delay"`

	// MaxRetries is the maximum number of retry attempts for the job in case of failure
	MaxRetries int				`json:"max_retries"`

	// RetryCount is the number of times the job has been retried after failure
	RetryCount int				`json:"retry_count"`

	// Error contains the error message if the job has failed, empty otherwise
	Error      string			`json:"error,omitempty"`

	// CreatedAt is the timestamp when the job was created
	CreatedAt  time.Time		`json:"created_at"`

	// UpdatedAt is the timestamp when the job was last updated, such as status change or retry attempt
	UpdatedAt  time.Time		`json:"updated_at"`

	// RunAfter is the timestamp after which the job becomes eligible for processing, calculated as CreatedAt + Delay
	RunAfter   time.Time		`json:"run_after"`
}

func wrapJob(job *internalJob.Job) *Job {
	if job == nil {
		return nil
	}

	return &Job{
		ID: job.ID,
		Type: job.Type,
		Payload: job.Payload,
		Status: Status(job.Status),
		Priority: job.Priority,
		Delay: job.Delay,
		MaxRetries: job.MaxRetries,
		RetryCount: job.RetryCount,
		Error: job.Error,
		CreatedAt: job.CreatedAt,
		UpdatedAt: job.UpdatedAt,
		RunAfter: job.RunAfter,
	}
}