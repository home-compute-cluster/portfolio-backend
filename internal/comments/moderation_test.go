package comments

import (
	"context"
	"errors"
	"testing"
)

func TestModerationListValidatesFilterAndPagination(t *testing.T) {
	t.Parallel()

	store := &recordingModerationStore{}
	service := NewModerationService(store, testLimits())
	if _, err := service.List(context.Background(), "removed", 0, 10); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("invalid status error = %v, want ErrInvalidStatus", err)
	}
	if _, err := service.List(context.Background(), StatusVisible, -1, 10); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("invalid cursor error = %v, want ErrInvalidCursor", err)
	}
	if _, err := service.List(context.Background(), StatusHidden, 0, 0); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if store.limit != 25 || store.status != StatusHidden {
		t.Fatalf("moderation query = status %q, limit %d", store.status, store.limit)
	}
}

func TestModerationUsesExplicitDesiredState(t *testing.T) {
	t.Parallel()

	store := &recordingModerationStore{}
	service := NewModerationService(store, testLimits())
	if _, err := service.Hide(context.Background(), 4); err != nil {
		t.Fatalf("Hide() error = %v", err)
	}
	if store.status != StatusHidden {
		t.Fatalf("Hide desired status = %q", store.status)
	}
	if _, err := service.Unhide(context.Background(), 4); err != nil {
		t.Fatalf("Unhide() error = %v", err)
	}
	if store.status != StatusVisible {
		t.Fatalf("Unhide desired status = %q", store.status)
	}
}

type recordingModerationStore struct {
	status Status
	limit  int
}

func (store *recordingModerationStore) ListForModeration(
	_ context.Context,
	status Status,
	_ int64,
	limit int,
) ([]Comment, error) {
	store.status = status
	store.limit = limit
	return nil, nil
}

func (store *recordingModerationStore) SetVisibility(
	_ context.Context,
	_ int64,
	status Status,
	_ int,
) (Comment, error) {
	store.status = status
	return Comment{Status: status}, nil
}
