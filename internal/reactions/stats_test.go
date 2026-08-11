package reactions

import (
	"context"
	"errors"
	"testing"
)

func TestStatsServiceReturnsStoreTotals(t *testing.T) {
	t.Parallel()

	want := Stats{Views: 12, Likes: 4}
	service := NewStatsService(staticStatsStore{stats: want}, acceptingContent{})
	got, err := service.Get(context.Background(), "known-post")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != want {
		t.Fatalf("Get() = %#v, want %#v", got, want)
	}
}

func TestStatsServiceRejectsUnknownContent(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("unknown post")
	service := NewStatsService(staticStatsStore{}, rejectingContent{err: wantErr})
	if _, err := service.Get(context.Background(), "unknown-post"); !errors.Is(err, wantErr) {
		t.Fatalf("Get() error = %v, want %v", err, wantErr)
	}
}

type staticStatsStore struct {
	stats Stats
	err   error
}

func (store staticStatsStore) GetStats(context.Context, string) (Stats, error) {
	return store.stats, store.err
}
