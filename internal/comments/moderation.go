package comments

import (
	"context"
	"errors"
)

var ErrInvalidStatus = errors.New("invalid comment status")

var ErrInvalidAuditActor = errors.New("invalid moderation audit actor")

// AuditContext contains only validated, non-sensitive metadata for a mutation audit event.
type AuditContext struct {
	ActorID   string
	RequestID string
}

type ModerationStore interface {
	ListForModeration(ctx context.Context, status Status, beforeID int64, limit int) ([]Comment, error)
	SetVisibility(
		ctx context.Context,
		id int64,
		status Status,
		maxVisible int,
		audit AuditContext,
	) (Comment, error)
}

type ModerationService struct {
	store  ModerationStore
	limits Limits
}

func NewModerationService(store ModerationStore, limits Limits) *ModerationService {
	return &ModerationService{store: store, limits: limits}
}

func (service *ModerationService) List(
	ctx context.Context,
	status Status,
	beforeID int64,
	limit int,
) ([]Comment, error) {
	if status != "" && status != StatusVisible && status != StatusHidden {
		return nil, ErrInvalidStatus
	}
	if beforeID < 0 || limit < 0 || limit > service.limits.MaximumPageSize {
		return nil, ErrInvalidCursor
	}
	if limit == 0 {
		limit = service.limits.DefaultPageSize
	}
	return service.store.ListForModeration(ctx, status, beforeID, limit)
}

func (service *ModerationService) Hide(
	ctx context.Context,
	id int64,
	audit AuditContext,
) (Comment, error) {
	if id <= 0 {
		return Comment{}, ErrNotFound
	}
	if !validAuditContext(audit) {
		return Comment{}, ErrInvalidAuditActor
	}
	return service.store.SetVisibility(ctx, id, StatusHidden, service.limits.MaxCommentsPerPost, audit)
}

func (service *ModerationService) Unhide(
	ctx context.Context,
	id int64,
	audit AuditContext,
) (Comment, error) {
	if id <= 0 {
		return Comment{}, ErrNotFound
	}
	if !validAuditContext(audit) {
		return Comment{}, ErrInvalidAuditActor
	}
	return service.store.SetVisibility(ctx, id, StatusVisible, service.limits.MaxCommentsPerPost, audit)
}

func validAuditContext(audit AuditContext) bool {
	return len(audit.ActorID) >= 1 && len(audit.ActorID) <= 255 && len(audit.RequestID) <= 128
}
