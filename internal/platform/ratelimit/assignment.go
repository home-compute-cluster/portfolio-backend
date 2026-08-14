package ratelimit

import (
	"sync"
	"time"
)

// AssignmentLimiter is intentionally incomplete. Implement a pod-local,
// concurrency-safe limiter here, then wire it in internal/app after the
// assignment acceptance tests pass.
type AssignmentLimiter struct {
	limit   int
	window  time.Duration
	maxKeys int
	// TODO: add bounded state and synchronization appropriate to your algorithm.
	mu   sync.Mutex
	user map[[32]byte]*VisitorState
}

// VisitorState records the per visitor status
type VisitorState struct {
	windowStart time.Time
	count       int
}

func NewAssignmentLimiter(limit int, window time.Duration, maxKeys int) *AssignmentLimiter {
	return &AssignmentLimiter{
		limit:   limit,
		window:  window,
		maxKeys: maxKeys,
		user:    make(map[[32]byte]*VisitorState),
	}
}

func (limiter *AssignmentLimiter) Allow(visitor [32]byte, now time.Time) bool {
	// TODO: implement this method. The temporary permissive result keeps the
	// normal backend build usable, but this limiter is not wired into the API.
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	if limiter.limit <= 0 ||
		limiter.window <= 0 ||
		limiter.maxKeys <= 0 {
		return false
	}

	if limiter.user == nil {
		limiter.user = make(map[[32]byte]*VisitorState)
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

	// Only perform the sweep when a new visitor exceed the retention bound
	if len(limiter.user) >= limiter.maxKeys {
		limiter.removeExpiredLock(now)

		// DO NOT evict active entries
		if len(limiter.user) >= limiter.maxKeys {
			return false
		}
	}

	limiter.user[visitor] = &VisitorState{
		windowStart: now,
		count:       1,
	}

	return true
}

func (limiter *AssignmentLimiter) removeExpiredLock(now time.Time) {
	for visitor, state := range limiter.user {
		if now.Sub(state.windowStart) >= limiter.window {
			delete(limiter.user, visitor)
		}
	}
}

// Size exists so the assignment tests can verify bounded key retention.
func (limiter *AssignmentLimiter) Size() int {
	// TODO: return the number of visitor keys currently retained.
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	return len(limiter.user)
}
