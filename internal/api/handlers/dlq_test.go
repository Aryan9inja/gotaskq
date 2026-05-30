package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/dlq"
	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/queue"
	testutils "github.com/Aryan9inja/gotaskq/internal/testUtils"
	"github.com/Aryan9inja/gotaskq/pkg/snowflake"
)

func TestListDeadJobs(t *testing.T) {
	client := testutils.GetRedisClient(t)
	t.Run("Success", func(t *testing.T) {
		dlqStore, err := dlq.NewRedisDlq(client)
		if err != nil {
			t.Fatalf("failed to create DLQ store: %v", err)
		}

		h := New(nil, nil, nil, dlqStore, nil, nil)
		req := httptest.NewRequest(http.MethodGet, "/dlq", nil)
		rr := httptest.NewRecorder()

		h.ListDeadJobs(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("DLQ store not configured", func(t *testing.T) {
		h := New(nil, nil, nil, nil, nil, nil)
		req := httptest.NewRequest(http.MethodGet, "/dlq", nil)
		rr := httptest.NewRecorder()

		h.ListDeadJobs(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
		}
	})
}

func TestReplayDeadJob(t *testing.T) {
	client := testutils.GetRedisClient(t)
	st, _ := job.NewRedisStore(client, 5*time.Minute)
	qm := queue.NewQueueManager()
	q := queue.NewMemoryQueue("default")
	qm.Register(q)
	dlqStore, err := dlq.NewRedisDlq(client)
	if err != nil {
		t.Fatalf("failed to create DLQ store: %v", err)
	}
	idGen := snowflake.New(5)

	h := New(st, qm, idGen, dlqStore, nil, nil)

	t.Run("Success", func(t *testing.T) {
		deadJob := testutils.NewTestJob("123", 5, 0)
		deadJob.Status = job.StatusDead

		dlqStore.Save(context.Background(), deadJob)

		req := testutils.RequestWithRouteParam(http.MethodPost, "/dlq/replay/123", "id", "123", nil)
		rr := httptest.NewRecorder()

		h.ReplayDeadJob(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("DLQ store not configured", func(t *testing.T) {
		h := New(nil, nil, nil, nil, nil, nil)
		req := testutils.RequestWithRouteParam(http.MethodPost, "/dlq/replay/123", "id", "123", nil)
		rr := httptest.NewRecorder()

		h.ReplayDeadJob(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
		}
	})
}

func TestDeleteDeadJob(t *testing.T) {
	client := testutils.GetRedisClient(t)

	t.Run("Success", func(t *testing.T) {
		deadJob := testutils.NewTestJob("123", 5, 0)
		deadJob.Status = job.StatusDead

		dlqStore, err := dlq.NewRedisDlq(client)
		if err != nil {
			t.Fatalf("failed to create DLQ store: %v", err)
		}

		dlqStore.Save(context.Background(), deadJob)

		h := New(nil, nil, nil, dlqStore, nil, nil)
		req := testutils.RequestWithRouteParam(http.MethodDelete, "/dlq/123", "id", "123", nil)
		rr := httptest.NewRecorder()

		h.DeleteDeadJob(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("DLQ store not configured", func(t *testing.T) {
		h := New(nil, nil, nil, nil, nil, nil)
		req := testutils.RequestWithRouteParam(http.MethodDelete, "/dlq/123", "id", "123", nil)
		rr := httptest.NewRecorder()

		h.DeleteDeadJob(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
		}
	})
}
