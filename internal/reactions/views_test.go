package reactions

import (
	"context"
	"testing"
	"time"
)

func TestRecordUsesClockAndRollingWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	store := &recordingViewStore{}
	service := NewViewService(store, acceptingContent{}, fixedClock{now}, 24*time.Hour)
	counted, err := service.Record(context.Background(), "known-post", [32]byte{1})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if !counted || store.countedAt != now || store.window != 24*time.Hour || store.slug != "known-post" {
		t.Fatalf("recorded view = %#v", store)
	}
}

type acceptingContent struct{}

func (acceptingContent) RequirePublishedContent(context.Context, string) error { return nil }

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

type recordingViewStore struct {
	slug      string
	countedAt time.Time
	window    time.Duration
}

func (store *recordingViewStore) RecordView(
	_ context.Context,
	postSlug string,
	_ [32]byte,
	countedAt time.Time,
	window time.Duration,
) (bool, error) {
	store.slug = postSlug
	store.countedAt = countedAt
	store.window = window
	return true, nil
}
