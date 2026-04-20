package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/go-chi/chi/v5"
)

type CreateJobRequest struct {
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	Priority   int             `json:"priority"` // 0 - 10
	Delay      time.Duration   `json:"delay"`    // seconds
	MaxRetries int             `json:"max_retries"`
}

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req CreateJobRequest

	q, err := h.queueManager.DefaultQueue()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "can't get default queue")
		return
	}

	// Validate body json
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}

	// Validate the job data
	if req.Type == "" {
		WriteError(w, http.StatusBadRequest, "type is required")
		return
	}

	if req.Priority < 0 || req.Priority > 10 {
		WriteError(w, http.StatusBadRequest, "priority must be between 0 and 10")
		return
	}

	// build the job
	now := time.Now()

	job := &job.Job{
		ID:         strconv.FormatInt(h.idGenerator.NextID(), 10),
		Type:       req.Type,
		Payload:    req.Payload,
		Priority:   req.Priority,
		Status:     job.StatusPending,
		MaxRetries: req.MaxRetries,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if req.Delay > 0 {
		job.RunAfter = now.Add(time.Duration(req.Delay) * time.Second)
	}

	err = h.store.Save(r.Context(), job)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to save job")
		return
	}

	err = q.Enqueue(r.Context(), job)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "enqueue failed")
		return
	}

	WriteJSON(w, http.StatusCreated, job)
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	job, err := h.store.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "can't get job with this id")
		return
	}

	WriteJSON(w, http.StatusOK, job)
}
