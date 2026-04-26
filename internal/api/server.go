package api

import (
	"net/http"

	"github.com/Aryan9inja/gotaskq/internal/api/handlers"
	"github.com/Aryan9inja/gotaskq/internal/dlq"
	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/queue"
	"github.com/Aryan9inja/gotaskq/pkg/snowflake"
	"github.com/go-chi/chi/v5"
)

type Server struct{
	store job.Store
	queueManager *queue.QueueManager
	idGenerator *snowflake.Snowflake
	dlqStore dlq.DlqInterface
}

func NewServer(st job.Store, qm *queue.QueueManager, idGen *snowflake.Snowflake, dlqStore dlq.DlqInterface) *Server{
	return &Server{
		store: st,
		queueManager: qm,
		idGenerator: idGen,
		dlqStore: dlqStore,
	}
}

func (s *Server) Start(addr string) error{
	r := chi.NewRouter()
	h := handlers.New(s.store, s.queueManager, s.idGenerator, s.dlqStore)

	// Route definition here
	r.Post("/jobs", h.CreateJob)
	r.Get("/jobs/{id}", h.GetJob)
	r.Get("/dlq", h.ListDeadJobs)
	r.Post("/dlq/{id}/replay", h.ReplayDeadJob)
	r.Delete("/dlq/{id}", h.DeleteDeadJob)

	return http.ListenAndServe(addr, r)
}

