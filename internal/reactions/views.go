package reactions

import (
	"context"
	"time"
)

type ViewStore interface {
	RecordView(
		ctx context.Context,
		postSlug string,
		visitorHash [32]byte,
		countedAt time.Time,
		deduplicationWindow time.Duration,
	) (bool, error)
}

type ContentRegistry interface {
	RequirePublishedPost(ctx context.Context, slug string) error
}

type Clock interface {
	Now() time.Time
}

type ViewService struct {
	store               ViewStore
	content             ContentRegistry
	clock               Clock
	deduplicationWindow time.Duration
}

func NewViewService(
	store ViewStore,
	content ContentRegistry,
	clock Clock,
	deduplicationWindow time.Duration,
) *ViewService {
	return &ViewService{
		store:               store,
		content:             content,
		clock:               clock,
		deduplicationWindow: deduplicationWindow,
	}
}

func (service *ViewService) Record(
	ctx context.Context,
	postSlug string,
	visitorHash [32]byte,
) (bool, error) {
	if err := service.content.RequirePublishedPost(ctx, postSlug); err != nil {
		return false, err
	}
	return service.store.RecordView(
		ctx,
		postSlug,
		visitorHash,
		service.clock.Now(),
		service.deduplicationWindow,
	)
}
