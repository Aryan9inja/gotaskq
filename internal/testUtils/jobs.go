package testutils

import (
	"encoding/json"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/job"
)

func NewTestJob(id string, priority int, delay time.Duration) *job.Job {
	now := time.Now()

	payload, _ := json.Marshal(map[string]any{"data": "test"})
	return &job.Job{
		ID:         id,
		Type:       "test",
		Payload:    payload,
		Priority:   priority,
		CreatedAt:  now,
		Delay:      delay,
		RunAfter:   now.Add(delay),
		RetryCount: 0,
		MaxRetries: 3,
		Status:     job.StatusPending,
		UpdatedAt:  now,
	}
}
