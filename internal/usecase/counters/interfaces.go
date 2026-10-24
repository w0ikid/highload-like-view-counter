package counters

import (
	"context"
	"highload-counters/internal/domain"
)

type Repository interface {
	SetLike(context.Context, int64, int64, bool) error
	RequirePosts(context.Context, []int64) error
	GetCounters(context.Context, int64, []int64) ([]domain.Counter, error)
}
type ViewStore interface {
	Record(context.Context, []domain.ViewEvent) (domain.RecordViewsResult, error)
}
