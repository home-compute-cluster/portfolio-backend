package reactions

import "context"

// LikeStore persists the desired like state for a visitor and post.
type LikeStore interface {
	AddLike(ctx context.Context, postSlug string, visitorHash [32]byte) (bool, error)
	RemoveLike(ctx context.Context, postSlug string, visitorHash [32]byte) (bool, error)
}

// LikeService validates content identity before changing a visitor's like state.
type LikeService struct {
	store   LikeStore
	content ContentRegistry
}

// NewLikeService constructs the application service for desired-state like operations.
func NewLikeService(store LikeStore, content ContentRegistry) *LikeService {
	return &LikeService{store: store, content: content}
}

// Like ensures the visitor likes the published post and reports whether storage changed.
func (service *LikeService) Like(
	ctx context.Context,
	postSlug string,
	visitorHash [32]byte,
) (bool, error) {
	if err := service.content.RequirePublishedPost(ctx, postSlug); err != nil {
		return false, err
	}
	return service.store.AddLike(ctx, postSlug, visitorHash)
}

// Unlike ensures the visitor does not like the published post and reports whether storage changed.
func (service *LikeService) Unlike(
	ctx context.Context,
	postSlug string,
	visitorHash [32]byte,
) (bool, error) {
	if err := service.content.RequirePublishedPost(ctx, postSlug); err != nil {
		return false, err
	}
	return service.store.RemoveLike(ctx, postSlug, visitorHash)
}
