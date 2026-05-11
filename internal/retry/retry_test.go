package retry

import (
	"testing"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/job"
)

func TestShouldRetry(t *testing.T) {
	cases := []struct {
		name string
		job  *job.Job
		want bool
	}{
		{
			name: "Should retry when retry count is less than maxRetries",
			job: &job.Job{
				RetryCount: 0,
				MaxRetries: 3,
			},
			want: true,
		},
		{
			name: "Should not retry when retry count equals maxRetries",
			job: &job.Job{
				RetryCount: 3,
				MaxRetries: 3,
			},
			want: false,
		},
		{
			name: "Should not retry when retry count exceeds maxRetries",
			job: &job.Job{
				RetryCount: 4,
				MaxRetries: 3,
			},
			want: false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := ShouldRetry(testCase.job); got != testCase.want {
				t.Errorf("ShouldRetry() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestNextDelay(t *testing.T) {
	engine := &RetryEngine{MaxDelay: 10 * time.Second}
	j := &job.Job{
		Delay:      1 * time.Second,
		RetryCount: 1,
	}

	delay := engine.NextDelay(j)

	// Check if delay is at least the exponential backoff (2s) and within jitter range (upto 2s + 0.4*2 = 2.8s)
	if delay < 2*time.Second || delay > 2800*time.Millisecond {
		t.Errorf("Next Delay() = %v, want range [2s, 2.8s]", delay)
	}

	// Check if delay > maxDelay
	j.RetryCount = 10 // 1 * 2^10 = 1024s
	delay = engine.NextDelay(j)
	if delay < 10*time.Second || delay >14*time.Second{
		t.Errorf("NextDelay() result = (%v), want in range [10s, 14s]", delay)
	}
}
