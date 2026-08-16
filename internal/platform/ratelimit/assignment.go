package ratelimit

import (
	"sync"
	"time"
)

// AssignmentLimiter applies a concurrency-safe fixed-window allowance per
// pseudonymous visitor. Its bounded state is local to one application pod.
type AssignmentLimiter struct {
	limit   int
	window  time.Duration
	maxKeys int
	mu      sync.Mutex
	user    map[[32]byte]*visitorState
}

// visitorState records one visitor's current fixed window and request count.
type visitorState struct {
	windowStart time.Time
	count       int
}

// NewAssignmentLimiter creates a limiter with the given per-window allowance
// and visitor retention bound. Invalid values produce a limiter that fails
// closed when Allow is called.
func NewAssignmentLimiter(limit int, window time.Duration, maxKeys int) *AssignmentLimiter {
	return &AssignmentLimiter{
		limit:   limit,
		window:  window,
		maxKeys: maxKeys,
		user:    make(map[[32]byte]*visitorState),
	}
}

// Allow reports whether the visitor has remaining allowance in its current
// fixed window. A new visitor is denied when maxKeys active entries are held.
func (limiter *AssignmentLimiter) Allow(visitor [32]byte, now time.Time) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	if limiter.limit <= 0 ||
		limiter.window <= 0 ||
		limiter.maxKeys <= 0 {
		return false
	}

	if limiter.user == nil {
		limiter.user = make(map[[32]byte]*visitorState)
	}

	if state, exists := limiter.user[visitor]; exists {
		if elapsed := now.Sub(state.windowStart); elapsed >= limiter.window {
			state.windowStart = now
			state.count = 1
			return true
		}

		if state.count >= limiter.limit {
			return false
		}

		state.count++
		return true
	}

	// Sweep only when admitting a new visitor would exceed the retention bound.
	if len(limiter.user) >= limiter.maxKeys {
		limiter.removeExpiredLocked(now)

		// Active entries are never evicted to make room for an unseen visitor.
		if len(limiter.user) >= limiter.maxKeys {
			return false
		}
	}

	limiter.user[visitor] = &visitorState{
		windowStart: now,
		count:       1,
	}
	return true
}

// removeExpiredLocked removes expired visitor windows. The caller must hold mu.
func (limiter *AssignmentLimiter) removeExpiredLocked(now time.Time) {
	for visitor, state := range limiter.user {
		if now.Sub(state.windowStart) >= limiter.window {
			delete(limiter.user, visitor)
		}
	}
}

// Size returns the number of visitor keys currently retained.
func (limiter *AssignmentLimiter) Size() int {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	return len(limiter.user)
}
