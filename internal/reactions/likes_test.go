package reactions

import (
	"context"
	"errors"
	"testing"
)

func TestLikeServiceUsesExplicitDesiredState(t *testing.T) {
	t.Parallel()

	store := &recordingLikeStore{}
	service := NewLikeService(store, acceptingContent{})
	visitorHash := [32]byte{1}

	if _, err := service.Like(context.Background(), "known-post", visitorHash); err != nil {
		t.Fatalf("Like() error = %v", err)
	}
	if store.addCalls != 1 || store.removeCalls != 0 {
		t.Fatalf("after Like calls = add %d, remove %d", store.addCalls, store.removeCalls)
	}

	if _, err := service.Unlike(context.Background(), "known-post", visitorHash); err != nil {
		t.Fatalf("Unlike() error = %v", err)
	}
	if store.addCalls != 1 || store.removeCalls != 1 {
		t.Fatalf("after Unlike calls = add %d, remove %d", store.addCalls, store.removeCalls)
	}
}

func TestLikeServiceRejectsUnknownContentBeforePersistence(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("unknown post")
	store := &recordingLikeStore{}
	service := NewLikeService(store, rejectingContent{err: wantErr})
	if _, err := service.Like(context.Background(), "unknown-post", [32]byte{}); !errors.Is(err, wantErr) {
		t.Fatalf("Like() error = %v, want %v", err, wantErr)
	}
	if store.addCalls != 0 {
		t.Fatal("unknown content reached like persistence")
	}
}

type recordingLikeStore struct {
	addCalls    int
	removeCalls int
}

func (store *recordingLikeStore) AddLike(context.Context, string, [32]byte) (bool, error) {
	store.addCalls++
	return true, nil
}

func (store *recordingLikeStore) RemoveLike(context.Context, string, [32]byte) (bool, error) {
	store.removeCalls++
	return true, nil
}

type rejectingContent struct{ err error }

func (content rejectingContent) RequirePublishedContent(context.Context, string) error {
	return content.err
}
