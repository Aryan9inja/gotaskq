package snowflake

import (
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Run("Valid machine IDs", func(t *testing.T) {
		tests := []int64{0, 1, 512, 1023}

		for _, id := range tests {
			s := New(id)
			if s.machineId != id {
				t.Errorf("Expected machine id %d, got %d", id, s.machineId)
			}
		}
	})

	t.Run("Invalid Machine ID (-ve)", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic for negative ids")
			}
		}()
		New(-1)
	})

	t.Run("Invalid machine ID (bigger)", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic for negative ids")
			}
		}()
		New(1024)
	})
}

func TestNextID(t *testing.T) {
	s := New(1)

	t.Run("Monotonicity", func(t *testing.T) {
		id1 := s.NextID()
		id2 := s.NextID()

		if id1 <= 0 || id2 <= 0 {
			t.Errorf("IDs should be positive, got %d and %d", id1, id2)
		}

		if id1 >= id2 {
			t.Errorf("IDs should be monotonically increasing: %d <= %d", id2, id1)
		}
	})

	t.Run("Sequence Overflow", func(t *testing.T) {
		origNowMilli := nowMilli
		t.Cleanup(func() { nowMilli = origNowMilli })

		fakeNow := int64(1700000000000)
		calls := 0
		nowMilli = func() int64 {
			calls++
			if calls <= 2 {
				return fakeNow
			}
			return fakeNow + 1
		}

		s.lastStamp = fakeNow
		s.sequence = maxSequence - 1

		id1 := s.NextID()
		if s.sequence != maxSequence {
			t.Fatalf("Expected sequence to reach maxSequence, got %d", s.sequence)
		}

		id2 := s.NextID()
		if id2 <= id1 {
			t.Errorf("Expected ID to advance after sequence overflow, %d <= %d", id2, id1)
		}
		if s.sequence != 0 {
			t.Errorf("Expected sequence to wrap to 0 after overflow, got %d", s.sequence)
		}
		if s.lastStamp != fakeNow+1 {
			t.Errorf("Expected lastStamp to increase after overflow, got %d", s.sequence)
		}
	})

	t.Run("Concurrency Safety", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoRoutines := 10
		idsPerGoroutine := 1000
		idChan := make(chan int64, numGoRoutines*idsPerGoroutine)

		for range numGoRoutines {
			wg.Go(func() {
				for range idsPerGoroutine {
					idChan <- s.NextID()
				}
			})
		}

		wg.Wait()
		close(idChan)

		seen := make(map[int64]bool)
		for id := range idChan {
			if seen[id] {
				t.Errorf("Duplicate ID generated in concurrent environment: %d", id)
			}
			seen[id] = true
		}
	})
}

func TestClockDrift(t *testing.T) {
	s := New(1)
	t.Run("Small Clock Drift", func(t *testing.T) {
		now := currMilli()
		s.lastStamp = now + 2

		id := s.NextID()

		idTime := (id >> timestampShift) + epoch
		if idTime < s.lastStamp {
			t.Errorf("Expected ID time to be >= lastStamp during small drift, got %d < %d", idTime, s.lastStamp)
		}
	})

	t.Run("Larger Clock Drift", func(t *testing.T) {
		now := currMilli()
		s.lastStamp = now + 10

		start := time.Now()
		id := s.NextID()
		elapsed := time.Since(start)

		if elapsed < 10*time.Millisecond {
			t.Logf("Warning: Large clock drift waiting was short (%v), might be due to fast execution or scheduling", elapsed)
		}

		idTime := (id >> timestampShift) + epoch
		if idTime < s.lastStamp {
			t.Errorf("Expected ID time to be >= lastStamp after waiting, got %d < %d", idTime, s.lastStamp)
		}
	})
}
