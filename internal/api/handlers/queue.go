package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) CreateNewQueue(w http.ResponseWriter, r *http.Request) {
	queueName := chi.URLParam(r, "name")

	queueName = strings.ToLower(queueName)

	queue, err := h.queueFactory.New(queueName)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create new queue with name: %s", queueName))
	}

	err = h.queueManager.Register(queue)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf(
			"failed to register new queue:%v", err))
		return
	}

	WriteJSON(w, http.StatusCreated, "Success creating new queue")
}

func (h *Handler) ListQueues(w http.ResponseWriter, r *http.Request) {
	queues := h.queueManager.ListNames()
	WriteJSON(w, http.StatusOK, queues)
}
