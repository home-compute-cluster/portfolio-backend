//go:build assignment

package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestAssignmentEnforcesLimitAndResetsAfterWindow(t *testing.T) {
	limiter := NewAssignmentLimiter(3, time.Minute, 100)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	key := assignmentKey(1)
	for request := range 3 {
		if !limiter.Allow(key, now.Add(time.Duration(request)*time.Second)) {
			t.Fatalf("request %d was denied within limit", request+1)
		}
	}
	if limiter.Allow(key, now.Add(3*time.Second)) {
		t.Fatal("request above limit was allowed")
	}
	if !limiter.Allow(key, now.Add(time.Minute)) {
		t.Fatal("request was not allowed after the window elapsed")
	}
}

func TestAssignmentSeparatesVisitors(t *testing.T) {
	limiter := NewAssignmentLimiter(1, time.Minute, 100)
	now := time.Now()
	if !limiter.Allow(assignmentKey(1), now) || !limiter.Allow(assignmentKey(2), now) {
		t.Fatal("one visitor consumed another visitor's allowance")
	}
}

func TestAssignmentIsConcurrencySafe(t *testing.T) {
	const limit = 10
	limiter := NewAssignmentLimiter(limit, time.Minute, 100)
	now := time.Now()
	key := assignmentKey(1)

	start := make(chan struct{})
	results := make(chan bool, 100)
	var ready sync.WaitGroup
	ready.Add(100)
	for range 100 {
		go func() {
			ready.Done()
			<-start
			results <- limiter.Allow(key, now)
		}()
	}
	ready.Wait()
	close(start)

	allowed := 0
	for range 100 {
		if <-results {
			allowed++
		}
	}
	if allowed != limit {
		t.Fatalf("concurrent allowed count = %d, want %d", allowed, limit)
	}
}

func TestAssignmentBoundsRetainedKeys(t *testing.T) {
	const maximumKeys = 10
	limiter := NewAssignmentLimiter(1, time.Minute, maximumKeys)
	now := time.Now()
	for index := range 100 {
		limiter.Allow(assignmentKey(byte(index)), now)
	}
	if size := limiter.Size(); size > maximumKeys {
		t.Fatalf("retained keys = %d, maximum = %d", size, maximumKeys)
	}
}

func TestAssignmentSizeReportsRetainedVisitors(t *testing.T) {
	limiter := NewAssignmentLimiter(1, time.Minute, 10)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	for index := byte(1); index <= 3; index++ {
		if !limiter.Allow(assignmentKey(index), now) {
			t.Fatalf("visitor %d was unexpectedly denied", index)
		}
	}
	if size := limiter.Size(); size != 3 {
		t.Fatalf("retained keys = %d, want 3", size)
	}
}

func TestAssignmentRemovesExpiredVisitorsAtCapacity(t *testing.T) {
	limiter := NewAssignmentLimiter(1, time.Minute, 2)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	if !limiter.Allow(assignmentKey(1), now) || !limiter.Allow(assignmentKey(2), now) {
		t.Fatal("initial visitors were unexpectedly denied")
	}
	if limiter.Allow(assignmentKey(3), now) {
		t.Fatal("new visitor was allowed while all retained windows were active")
	}
	if !limiter.Allow(assignmentKey(3), now.Add(time.Minute)) {
		t.Fatal("new visitor was denied after retained windows expired")
	}
	if size := limiter.Size(); size != 1 {
		t.Fatalf("retained keys after cleanup = %d, want 1", size)
	}
}

func TestAssignmentInvalidConfigurationFailsClosed(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	for _, limiter := range []*AssignmentLimiter{
		NewAssignmentLimiter(0, time.Minute, 1),
		NewAssignmentLimiter(1, 0, 1),
		NewAssignmentLimiter(1, time.Minute, 0),
	} {
		if limiter.Allow(assignmentKey(1), now) {
			t.Fatal("invalid limiter configuration allowed a request")
		}
	}
}

func assignmentKey(value byte) [32]byte {
	var key [32]byte
	key[0] = value
	return key
}
