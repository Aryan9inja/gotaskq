package queue

import (
	"testing"
)

func TestMemoryFactory(t *testing.T) {
	factory := NewMemoryFactory()

	t.Run("Create Success", func(t *testing.T) {
		q, err := factory.New("test-q")
		if err != nil {
			t.Fatalf("Expected success, got error : %v", err)
		}
		if q.Name() != "test-q" {
			t.Fatalf("Expected queue name 'test-q', got '%s'", q.Name())
		}
		if _, ok := q.(*MemoryQueue); !ok {
			t.Fatalf("Expected type *MemoryQueue, got %T", q)
		}
	})

	t.Run("Empty Name", func(t *testing.T) {
		_, err := factory.New(" ")
		if err != ErrEmptyQueueName {
			t.Errorf("Expected ErrEmptyQueueName, got %v", err)
		}
	})
}
