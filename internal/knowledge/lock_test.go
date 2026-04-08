package knowledge

import (
	"sync"
	"testing"
)

func TestTryLock_SucceedsOnFirstCall(t *testing.T) {
	lm := NewLockManager()
	if !lm.TryLock("owner/repo/TICKET-1") {
		t.Fatal("expected TryLock to succeed on first call")
	}
}

func TestTryLock_FailsOnSecondCall(t *testing.T) {
	lm := NewLockManager()
	key := "owner/repo/TICKET-1"
	lm.TryLock(key)
	if lm.TryLock(key) {
		t.Fatal("expected TryLock to fail on second call with same key")
	}
}

func TestUnlock_AllowsRelocking(t *testing.T) {
	lm := NewLockManager()
	key := "owner/repo/TICKET-1"
	lm.TryLock(key)
	lm.Unlock(key)
	if !lm.TryLock(key) {
		t.Fatal("expected TryLock to succeed after Unlock")
	}
}

func TestTryLock_DifferentKeysDontInterfere(t *testing.T) {
	lm := NewLockManager()
	key1 := "owner/repo/TICKET-1"
	key2 := "owner/repo/TICKET-2"

	if !lm.TryLock(key1) {
		t.Fatal("expected TryLock to succeed for key1")
	}
	if !lm.TryLock(key2) {
		t.Fatal("expected TryLock to succeed for key2 while key1 is held")
	}
}

func TestTryLock_ConcurrentOnlyOneWins(t *testing.T) {
	lm := NewLockManager()
	key := "owner/repo/TICKET-RACE"

	const goroutines = 100
	var wg sync.WaitGroup
	wins := make(chan bool, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			wins <- lm.TryLock(key)
		}()
	}
	wg.Wait()
	close(wins)

	winCount := 0
	for got := range wins {
		if got {
			winCount++
		}
	}
	if winCount != 1 {
		t.Fatalf("expected exactly 1 goroutine to win TryLock, got %d", winCount)
	}
}
