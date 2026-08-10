package ratelimit

import "time"

// Limiter is the boundary consumed by anonymous-write HTTP handlers.
// Implementations must be safe for concurrent use and bound retained keys.
type Limiter interface {
	Allow(visitorHash [32]byte, now time.Time) bool
}
