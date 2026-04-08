package knowledge

import "sync"

// LockManager manages per-ticket locks using a map of ticket keys.
// Key format: "owner/repo/TICKET-ID"
type LockManager struct {
	mu    sync.Mutex
	locks map[string]struct{}
}

// NewLockManager creates a new LockManager.
func NewLockManager() *LockManager {
	return &LockManager{
		locks: make(map[string]struct{}),
	}
}

// TryLock attempts to acquire a lock for the given key.
// Returns true if the lock was acquired, false if it is already held.
func (lm *LockManager) TryLock(key string) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if _, held := lm.locks[key]; held {
		return false
	}
	lm.locks[key] = struct{}{}
	return true
}

// Unlock releases the lock for the given key.
func (lm *LockManager) Unlock(key string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	delete(lm.locks, key)
}
