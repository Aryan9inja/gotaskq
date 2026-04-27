package metrics

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	jobsEnqueuedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gotaskq_jobs_enqueued_total",
		Help: "Total number of jobs enqueued",
	}, []string{"queue", "type"})

	jobsProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gotaskq_jobs_processed_total",
		Help: "Total number of jobs processed",
	}, []string{"queue", "type", "status"})

	jobDurationSecond = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "gotaskq_job_duration_seconds",
		Help: "Duration of job processing in seconds",
	}, []string{"queue", "type"})

	activeWorkers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gotaskq_active_workers",
		Help: "Current number of active workers",
	})

	queueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gotaskq_queue_depth",
		Help: "Current queue depth",
	}, []string{"queue"})

	jobsRetriedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gotaskq_jobs_retried_total",
		Help: "Total number of jobs retried",
	}, []string{"queue", "type"})

	jobsDeadTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gotaskq_jobs_dead_total",
		Help: "Total number of dead-letter jobs",
	}, []string{"queue", "type"})
)

func normalizeLabel(val string) string {
	label := strings.TrimSpace(strings.ToLower(val))
	if label == "" {
		label = "unknown"
	}

	return label
}

func IncJobsEnqueued(queueName, jobType string) {
	jobsEnqueuedTotal.WithLabelValues(normalizeLabel(queueName), normalizeLabel(jobType)).Inc()
}

func IncJobsProcessed(queueName, jobType, status string) {
	jobsProcessedTotal.WithLabelValues(
		normalizeLabel(queueName),
		normalizeLabel(jobType),
		normalizeLabel(status),
	).Inc()
}

func ObserveJobDuration(queueName, jobType string, duration time.Duration) {
	jobDurationSecond.WithLabelValues(
		normalizeLabel(queueName),
		normalizeLabel(jobType),
	).Observe(duration.Seconds())
}

func IncActiveWorkers() {
	activeWorkers.Inc()
}

func DecActiveWorkers() {
	activeWorkers.Dec()
}

func SetQueueDepth(queueName string, depth int) {
	queueDepth.WithLabelValues(normalizeLabel(queueName)).Set(float64(depth))
}

func IncJobsRetried(queueName, jobType string) {
	jobsRetriedTotal.WithLabelValues(normalizeLabel(queueName), normalizeLabel(jobType)).Inc()
}

func IncJobsDead(queueName, jobType string) {
	jobsDeadTotal.WithLabelValues(normalizeLabel(queueName), normalizeLabel(jobType)).Inc()
}
