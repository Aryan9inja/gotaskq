package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/queue"
	testutils "github.com/Aryan9inja/gotaskq/internal/testUtils"
	"github.com/Aryan9inja/gotaskq/pkg/snowflake"
)

func TestCreateJob(t *testing.T) {
	st := job.NewMemoryStore()
	qm := queue.NewQueueManager()
	q := queue.NewMemoryQueue("default")
	qm.Register(q)
	idGen := snowflake.New(5)

	h := New(st, qm, idGen, nil, nil, nil)

	t.Run("Success", func(t *testing.T) {
		reqBody := `{"type":"email","priority":5}`
		req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(reqBody))
		rr := httptest.NewRecorder()

		h.CreateJob(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d", http.StatusCreated, rr.Code)
		}

		var createdJob job.Job
		testutils.DecodeJSON(t, rr.Body, &createdJob)

		if createdJob.Type != "email" || createdJob.Priority != 5 {
			t.Errorf("expected job type 'email' and priority 5, got '%s' and %d", createdJob.Type, createdJob.Priority)
		}

		if _, err := st.Get(context.Background(), createdJob.ID); err != nil {
			t.Fatalf("expected job to be stored: %v", err)
		}

		if q.Len() != 1 {
			t.Fatalf("expected job to be enqueued, queue length should be 1, got %d", q.Len())
		}
	})

	t.Run("Invalid request body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`invalid json`))
		rr := httptest.NewRecorder()

		h.CreateJob(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("Missing required fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{"priority":5}`))
		rr := httptest.NewRecorder()

		h.CreateJob(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("Invalid priority", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{"type":"email","priority":15}`))
		rr := httptest.NewRecorder()

		h.CreateJob(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})
}

func TestCreateJobOnQueue(t *testing.T) {
	st := job.NewMemoryStore()
	qm := queue.NewQueueManager()
	q := queue.NewMemoryQueue("custom")
	qm.Register(q)
	idGen := snowflake.New(5)
	
	h := New(st, qm, idGen, nil, nil, nil)

	t.Run("Success", func(t *testing.T) {
		reqBody := `{"type":"report","priority":3}`
		req := testutils.RequestWithRouteParam(http.MethodPost, "/queue/custom/jobs", "queue", "custom", bytes.NewBufferString(reqBody))
		rr := httptest.NewRecorder()

		h.CreateJobOnQueue(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d", http.StatusCreated, rr.Code)
		}

		var createdJob job.Job
		testutils.DecodeJSON(t, rr.Body, &createdJob)

		if createdJob.Type != "report" || createdJob.Priority != 3 {
			t.Errorf("expected job type 'report' and priority 3, got '%s' and %d", createdJob.Type, createdJob.Priority)
		}

		if _, err := st.Get(context.Background(), createdJob.ID); err != nil {
			t.Fatalf("expected job to be stored: %v", err)
		}

		if q.Len() != 1 {
			t.Fatalf("expected job to be enqueued, queue length should be 1, got %d", q.Len())
		}
	})

	t.Run("Queue not found", func(t *testing.T) {
		reqBody := `{"type":"report","priority":3}`
		req := testutils.RequestWithRouteParam(http.MethodPost, "/queue/nonexistent/jobs", "queue", "nonexistent", bytes.NewBufferString(reqBody))
		rr := httptest.NewRecorder()

		h.CreateJobOnQueue(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
		}
	})
}

func TestGetJob(t *testing.T){
	st:= job.NewMemoryStore()
	h := New(st, nil, nil, nil, nil, nil)

	t.Run("Success", func(t *testing.T) {
		j := &job.Job{
			ID: "123",
			Type: "email",
			Priority: 5,
		}
		st.Save(context.Background(), j)
		
		req := testutils.RequestWithRouteParam(http.MethodGet, "/jobs/123", "id", "123", nil)
		rr := httptest.NewRecorder()
		
		h.GetJob(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}

		var gotJob job.Job
		testutils.DecodeJSON(t, rr.Body, &gotJob)

		if gotJob.ID != "123" || gotJob.Type != "email" || gotJob.Priority != 5 {
			t.Errorf("expected job with ID '123', type 'email', priority 5, got ID '%s', type '%s', priority %d", gotJob.ID, gotJob.Type, gotJob.Priority)
		}
	})

	t.Run("Job not found", func(t *testing.T) {
		req := testutils.RequestWithRouteParam(http.MethodGet, "/jobs/nonexistent", "id", "nonexistent", nil)
		rr := httptest.NewRecorder()

		h.GetJob(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})
}