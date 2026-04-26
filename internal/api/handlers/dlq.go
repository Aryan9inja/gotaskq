package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/dlq"
	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/go-chi/chi/v5"
)

const dlqListLimit int64 = (1 << 62) - 1

func (h *Handler) ListDeadJobs(w http.ResponseWriter, r *http.Request) {
	if h.dlqStore == nil {
		WriteError(w, http.StatusInternalServerError, "dlq store is not configured")
		return
	}

	deadJobs, err := h.dlqStore.List(r.Context(), dlqListLimit)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list dead jobs")
		return
	}

	WriteJSON(w, http.StatusOK, deadJobs)
}

func (h *Handler) ReplayDeadJob(w http.ResponseWriter, r *http.Request) {
	if h.dlqStore == nil {
		WriteError(w, http.StatusInternalServerError, "dlq store is not configured")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "job id is required")
		return
	}

	q, err := h.queueManager.DefaultQueue()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get default queue")
		return
	}

	deadJob, err := h.dlqStore.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, dlq.ErrJobNotFound) {
			WriteError(w, http.StatusNotFound, "dead job not found")
			return
		}

		WriteError(w, http.StatusInternalServerError, "failed to get dead job")
		return
	}

	if deadJob.Status != job.StatusDead {
		WriteError(w, http.StatusBadRequest, "job is not in dead status")
		return
	}

	now := time.Now()
	deadJob.Status = job.StatusPending
	deadJob.UpdatedAt = now
	deadJob.Error = ""
	deadJob.RunAfter = now

	if err := h.store.Save(r.Context(), deadJob); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to save replayed job")
		return
	}

	if err := q.Enqueue(r.Context(), deadJob); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to enqueue replayed job")
		return
	}

	if err := h.dlqStore.Delete(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to delete replayed job from DLQ")
		return
	}

	WriteJSON(w, http.StatusOK, deadJob)
}

func (h Handler) DeleteDeadJob(w http.ResponseWriter, r *http.Request) {
	if h.dlqStore == nil {
		WriteError(w, http.StatusInternalServerError, "dlq store is not configured")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "job id is required")
		return
	}

	_, err := h.dlqStore.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, dlq.ErrJobNotFound) {
			WriteError(w, http.StatusNotFound, "dead job not found")
			return
		}

		WriteError(w, http.StatusInternalServerError, "failed to get dead job")
		return
	}

	if err := h.dlqStore.Delete(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to delete dead job")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"message": "dead job deleted successfully",
	})
}
