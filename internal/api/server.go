package api

import (
	"net/http"

	"github.com/Aryan9inja/gotaskq/internal/api/handlers"
	"github.com/Aryan9inja/gotaskq/internal/dlq"
	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/queue"
	"github.com/Aryan9inja/gotaskq/pkg/snowflake"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	store        job.Store
	queueManager *queue.QueueManager
	idGenerator  *snowflake.Snowflake
	dlqStore     dlq.DlqInterface
	queueFactory queue.Factory
}

func NewServer(st job.Store, qm *queue.QueueManager, idGen *snowflake.Snowflake, dlqStore dlq.DlqInterface, factory queue.Factory) *Server {
	return &Server{
		store:        st,
		queueManager: qm,
		idGenerator:  idGen,
		dlqStore:     dlqStore,
		queueFactory: factory,
	}
}

func (s *Server) Start(addr string) error {
	r := chi.NewRouter()
	h := handlers.New(s.store, s.queueManager, s.idGenerator, s.dlqStore, s.queueFactory)

	// Route definition here
	r.Handle("/metrics", promhttp.Handler())
	r.Post("/jobs", h.CreateJob)
	r.Get("/jobs/{id}", h.GetJob)
	r.Post("/{queue}/jobs",h.CreateJobOnQueue)
	r.Get("/dlq", h.ListDeadJobs)
	r.Post("/dlq/{id}/replay", h.ReplayDeadJob)
	r.Delete("/dlq/{id}", h.DeleteDeadJob)
	r.Post("/queue/{name}", h.CreateNewQueue)
	r.Get("/queue", h.ListQueues)

	return http.ListenAndServe(addr, r)
}
