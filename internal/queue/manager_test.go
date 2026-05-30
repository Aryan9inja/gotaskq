package queue

import (
	"testing"
)

func TestQueueManager(t *testing.T) {
	manager := NewQueueManager()
	t.Run("Register and Get Queue", func(t *testing.T) {
		queue := NewMemoryQueue("q1")

		if err := manager.Register(queue); err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		got, err := manager.Get("q1")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got != queue {
			t.Fatalf("Retrieved queue does not match registered queue")
		}
	})

	t.Run("Register Validation", func(t *testing.T) {
		if err := manager.Register(nil); err != ErrNilQueue {
			t.Fatalf("Expected ErrNilQueue, got: %v", err)
		}

		if err := manager.Register(NewMemoryQueue("")); err != ErrEmptyQueueName {
			t.Fatalf("Expected ErrEmptyQueueName, got: %v", err)
		}

		queue := NewMemoryQueue("q1")
		manager.Register(queue)
		if err := manager.Register(queue); err != ErrQueueAlreadyExists {
			t.Fatalf("Expected ErrQueueAlreadyExists, got: %v", err)
		}
	})

	t.Run("Default Queue", func(t *testing.T) {
		localManager := NewQueueManager()
		if _, err := localManager.DefaultQueue(); err != ErrDefaultQueueNotSet {
			t.Fatalf("Expected ErrDefaultQueueNotSet, got : %v", err)
		}

		defaultQueue := NewMemoryQueue("default")
		localManager.Register(defaultQueue)
		if localManager.DefaultName() != "default" {
			t.Fatalf("Expected default queue name to be 'default', got: %s", localManager.DefaultName())
		}

		// Manual Override
		q1 := NewMemoryQueue("q1")
		localManager.Register(q1)
		if err := localManager.SetDefault("q1"); err != nil {
			t.Fatalf("SetDefault failed: %v", err)
		}
		if localManager.DefaultName() != "q1" {
			t.Fatalf("Expected default queue name to be 'q1', got: %s", localManager.DefaultName())
		}

		// Set invalid default
		if err := localManager.SetDefault("nonexistent"); err != ErrQueueNotFound {
			t.Fatalf("Expected ErrQueueNotFound, got: %v", err)
		}
	})

	t.Run("List Names", func(t *testing.T) {
		localManager := NewQueueManager()
		localManager.Register(NewMemoryQueue("q1"))
		localManager.Register(NewMemoryQueue("q2"))

		names := localManager.ListNames()
		if len(names) != 2 {
			t.Fatalf("Expected 2 queue names, got: %d", len(names))
		}
		if names[0] != "q1" && names[1] != "q2" {
			t.Fatalf("Expected queue names to be 'q1' and 'q2', got: %v", names)
		}
	})
}
