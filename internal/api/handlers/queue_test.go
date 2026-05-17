package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Aryan9inja/gotaskq/internal/queue"
	testutils "github.com/Aryan9inja/gotaskq/internal/testUtils"
)

type recordingRegistrar struct {
	count int
}

func (r *recordingRegistrar) AddQueue(q queue.Queue) {
	r.count++
}

func requestWithQueueParam(name string) *http.Request {
	return testutils.RequestWithRouteParam(http.MethodPost, "/queue/"+name, "name", name, nil)
}

func TestCreateNewQueue(t *testing.T) {
	t.Run("add queue to registrar after successfull registration", func(t *testing.T) {
		manager := queue.NewQueueManager()
		registrar := &recordingRegistrar{}
		handler := New(nil, manager, nil, nil, queue.NewMemoryFactory(), registrar)

		rr := httptest.NewRecorder()
		handler.CreateNewQueue(rr, requestWithQueueParam("xyz"))

		if rr.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d", http.StatusCreated, rr.Code)
		}
		if registrar.count != 1 {
			t.Fatalf("expected registrar to be called once, got %d calls", registrar.count)
		}
		if _, err := manager.Get("xyz"); err != nil {
			t.Fatalf("expected queue to be registered: %v", err)
		}
	})

	t.Run("does not add queue to registrar when registration fails", func(t *testing.T) {
		manager := queue.NewQueueManager()
		if err := manager.Register(queue.NewMemoryQueue("xyz")); err != nil {
			t.Fatalf("register existing queue: %v", err)
		}

		registrar := &recordingRegistrar{}
		handler := New(nil, manager, nil, nil, queue.NewMemoryFactory(), registrar)

		rr := httptest.NewRecorder()
		handler.CreateNewQueue(rr, requestWithQueueParam("xyz"))

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
		}
		if registrar.count != 0 {
			t.Fatalf("expected registrar not to be called, got %d calls", registrar.count)
		}
	})
}
