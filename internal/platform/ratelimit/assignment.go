package ratelimit

import "time"

// AssignmentLimiter is intentionally incomplete. Implement a pod-local,
// concurrency-safe limiter here, then wire it in internal/app after the
// assignment acceptance tests pass.
type AssignmentLimiter struct {
	limit   int
	window  time.Duration
	maxKeys int
	// TODO: add bounded state and synchronization appropriate to your algorithm.
}

func NewAssignmentLimiter(limit int, window time.Duration, maxKeys int) *AssignmentLimiter {
	return &AssignmentLimiter{limit: limit, window: window, maxKeys: maxKeys}
}

func (limiter *AssignmentLimiter) Allow(_ [32]byte, _ time.Time) bool {
	// TODO: implement this method. The temporary permissive result keeps the
	// normal backend build usable, but this limiter is not wired into the API.
	return true
}

// Size exists so the assignment tests can verify bounded key retention.
func (limiter *AssignmentLimiter) Size() int {
	// TODO: return the number of visitor keys currently retained.
	return 0
}
