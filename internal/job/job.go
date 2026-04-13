package job

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusPending Status = "PENDING"
	StatusRunning Status = "RUNNING"
	StatusDone    Status = "DONE"
	StatusFailed  Status = "FAILED"
	StatusDead    Status = "DEAD"
)

type Job struct {
	ID         string			`json:"id"`
	Type       string			`json:"type"`
	Payload    json.RawMessage	`json:"payload"`
	Status     Status			`json:"status"`
	Priority   int				`json:"priority"`
	Delay      time.Duration	`json:"delay"`
	MaxRetries int				`json:"max_retries"`
	RetryCount int				`json:"retry_count"`
	Error      string			`json:"error,omitempty"`
	CreatedAt  time.Time		`json:"created_at"`
	UpdatedAt  time.Time		`json:"updated_at"`
	RunAfter   time.Time		`json:"run_after"`
}
