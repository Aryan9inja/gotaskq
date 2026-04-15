package handler

import (
	"encoding/json"
	"net/http"

	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/queue"
	"github.com/Aryan9inja/gotaskq/pkg/snowflake"
)

type Handler struct {
	store        job.Store
	queueManager *queue.QueueManager
	idGenerator  *snowflake.Snowflake
}

func New(st job.Store, qm *queue.QueueManager, idGen *snowflake.Snowflake) *Handler {
	return &Handler{
		store:        st,
		queueManager: qm,
		idGenerator:  idGen,
	}
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{
		"error": msg,
	})
}
