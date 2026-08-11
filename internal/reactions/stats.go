package reactions

import "context"

// Stats contains the public aggregate reaction totals for one post.
type Stats struct {
	Views int64
	Likes int64
}

// StatsStore reads aggregate post statistics.
type StatsStore interface {
	GetStats(ctx context.Context, postSlug string) (Stats, error)
}

// StatsService validates content identity before exposing aggregate statistics.
type StatsService struct {
	store   StatsStore
	content ContentRegistry
}

// NewStatsService constructs the public statistics application service.
func NewStatsService(store StatsStore, content ContentRegistry) *StatsService {
	return &StatsService{store: store, content: content}
}

// Get returns aggregate statistics for a published post.
func (service *StatsService) Get(ctx context.Context, postSlug string) (Stats, error) {
	if err := service.content.RequirePublishedPost(ctx, postSlug); err != nil {
		return Stats{}, err
	}
	return service.store.GetStats(ctx, postSlug)
}
