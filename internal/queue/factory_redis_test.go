package queue_test

import (
	"testing"

	"github.com/Aryan9inja/gotaskq/internal/queue"
	testutils "github.com/Aryan9inja/gotaskq/internal/testUtils"
)

func TestRedisFactory(t *testing.T) {
	client := testutils.GetRedisClient(t)
	factory, err := queue.NewRedisFactory(client)
	if err != nil {
		t.Fatalf("Expected success, got error : %v", err)
	}

	t.Run("Create Success", func(t *testing.T) {
		q, err := factory.New("test-q")
		if err != nil {
			t.Fatalf("Expected success, got error : %v", err)
		}
		if q.Name() != "test-q" {
			t.Fatalf("Expected queue name 'test-q', got '%s'", q.Name())
		}
		if _, ok := q.(*queue.RedisQueue); !ok {
			t.Fatalf("Expected type *queue.RedisQueue, got %T", q)
		}
	})

	t.Run("Nil Client", func(t *testing.T) {
		_, err := queue.NewRedisFactory(nil)
		if err != queue.ErrRedisClientNil {
			t.Errorf("Expected ErrRedisClientNil, got %v", err)
		}
	})
}