package api

import (
	"net/http"

	"github.com/Aryan9inja/gotaskq/internal/api/handler"
	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/queue"
	"github.com/Aryan9inja/gotaskq/pkg/snowflake"
	"github.com/go-chi/chi/v5"
)

type Server struct{
	store job.Store
	queueManager *queue.QueueManager
	idGenerator *snowflake.Snowflake
}

func NewServer(st job.Store, qm *queue.QueueManager, idGen *snowflake.Snowflake) *Server{
	return &Server{
		store: st,
		queueManager: qm,
		idGenerator: idGen,
	}
}

func (s *Server) Start(addr string) error{
	r := chi.NewRouter()
	h := handler.New(s.store, s.queueManager, s.idGenerator)

	// Route definition here
	r.Post("/jobs", h.CreateJob)
	r.Get("/jobs/{id}", h.GetJob)

	return http.ListenAndServe(addr, r)
}

