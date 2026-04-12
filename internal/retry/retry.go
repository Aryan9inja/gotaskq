package retry

import "time"

type Engine struct{
	BaseDelay time.Duration
	MaxDelay time.Duration
	MaxRetries int
}