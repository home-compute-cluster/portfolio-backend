package postgres

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestIntegrationLikeAndUnlikeAreIdempotent(t *testing.T) {
	pool := migratedPool(t)
	store := NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	visitor := viewHash(1)

	changed, err := store.AddLike(ctx, testPost, visitor)
	if err != nil || !changed {
		t.Fatalf("first AddLike() = changed %t, error %v; want true, nil", changed, err)
	}
	changed, err = store.AddLike(ctx, testPost, visitor)
	if err != nil || changed {
		t.Fatalf("repeated AddLike() = changed %t, error %v; want false, nil", changed, err)
	}
	changed, err = store.RemoveLike(ctx, testPost, visitor)
	if err != nil || !changed {
		t.Fatalf("first RemoveLike() = changed %t, error %v; want true, nil", changed, err)
	}
	changed, err = store.RemoveLike(ctx, testPost, visitor)
	if err != nil || changed {
		t.Fatalf("repeated RemoveLike() = changed %t, error %v; want false, nil", changed, err)
	}
}

func TestIntegrationConcurrentLikesRemainUnique(t *testing.T) {
	pool := migratedPool(t)
	store := NewStore(pool)

	const requests = 30
	start := make(chan struct{})
	results := make(chan bool, requests)
	errorsFound := make(chan error, requests)
	var ready sync.WaitGroup
	ready.Add(requests)
	for range requests {
		go func() {
			ready.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			changed, err := store.AddLike(ctx, testPost, viewHash(1))
			results <- changed
			errorsFound <- err
		}()
	}
	ready.Wait()
	close(start)

	changedCount := 0
	for range requests {
		if err := <-errorsFound; err != nil {
			t.Fatalf("concurrent AddLike() error = %v", err)
		}
		if <-results {
			changedCount++
		}
	}
	if changedCount != 1 {
		t.Fatalf("concurrent changed count = %d, want 1", changedCount)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stats, err := store.GetStats(ctx, testPost)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.Likes != 1 {
		t.Fatalf("like count = %d, want 1", stats.Likes)
	}
}

func TestIntegrationStatsCombineViewsAndLikes(t *testing.T) {
	pool := migratedPool(t)
	reactionStore := NewStore(pool)
	viewStore := NewViewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)

	for visitor := byte(1); visitor <= 3; visitor++ {
		if _, err := viewStore.RecordView(ctx, testPost, viewHash(visitor), now, 24*time.Hour); err != nil {
			t.Fatalf("RecordView(visitor %d): %v", visitor, err)
		}
	}
	for visitor := byte(1); visitor <= 2; visitor++ {
		if _, err := reactionStore.AddLike(ctx, testPost, viewHash(visitor)); err != nil {
			t.Fatalf("AddLike(visitor %d): %v", visitor, err)
		}
	}

	stats, err := reactionStore.GetStats(ctx, testPost)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.Views != 3 || stats.Likes != 2 {
		t.Fatalf("stats = %#v, want 3 views and 2 likes", stats)
	}
}

func TestIntegrationLikeConstraintsRejectInvalidRows(t *testing.T) {
	pool := migratedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx,
		`INSERT INTO post_likes (post_slug, visitor_hash) VALUES ($1, $2)`,
		"unknown-post", make([]byte, 32),
	); err == nil {
		t.Fatal("like for unknown content was accepted")
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO post_likes (post_slug, visitor_hash) VALUES ($1, $2)`,
		testPost, make([]byte, 31),
	); err == nil {
		t.Fatal("like with invalid visitor hash was accepted")
	}
}
