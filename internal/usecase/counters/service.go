package counters

import (
	"context"
	"errors"
	"highload-counters/internal/domain"
	"time"
)

type Service struct {
	repo                          Repository
	views                         ViewStore
	postgresTimeout, redisTimeout time.Duration
}

func New(repo Repository, views ViewStore, pgTimeout, redisTimeout time.Duration) *Service {
	return &Service{repo, views, pgTimeout, redisTimeout}
}
func userValid(userID int64) error {
	if userID <= 0 {
		return domain.ErrUnauthorized
	}
	return nil
}
func (s *Service) SetLike(ctx context.Context, userID, postID int64, liked bool) error {
	if err := userValid(userID); err != nil {
		return err
	}
	if postID <= 0 {
		return domain.ErrPostIDInvalid
	}
	dependency, cancel := context.WithTimeout(ctx, s.postgresTimeout)
	defer cancel()
	return dependencyError(ctx, s.repo.SetLike(dependency, userID, postID, liked))
}
func (s *Service) RecordViews(ctx context.Context, userID int64, events []domain.ViewEvent) (domain.RecordViewsResult, error) {
	var zero domain.RecordViewsResult
	if err := userValid(userID); err != nil {
		return zero, err
	}
	if len(events) < 1 || len(events) > domain.MaxViewEvents {
		return zero, domain.ErrViewBatchInvalid
	}
	ids := make([]int64, 0, len(events))
	seen := map[int64]bool{}
	for _, event := range events {
		if event.PostID <= 0 || event.ViewerID <= 0 {
			return zero, domain.ErrViewEventInvalid
		}
		if !seen[event.PostID] {
			ids = append(ids, event.PostID)
			seen[event.PostID] = true
		}
	}
	pgCtx, cancel := context.WithTimeout(ctx, s.postgresTimeout)
	err := s.repo.RequirePosts(pgCtx, ids)
	cancel()
	if err != nil {
		return zero, dependencyError(ctx, err)
	}
	redisCtx, cancel := context.WithTimeout(ctx, s.redisTimeout)
	defer cancel()
	out, err := s.views.Record(redisCtx, events)
	return out, dependencyError(ctx, err)
}
func (s *Service) GetCounters(ctx context.Context, userID int64, ids []int64) ([]domain.Counter, error) {
	if err := userValid(userID); err != nil {
		return nil, err
	}
	if len(ids) < 1 || len(ids) > domain.MaxCounterPosts {
		return nil, domain.ErrCounterBatchInvalid
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return nil, domain.ErrCounterBatchInvalid
		}
		seen[id] = true
	}
	dependency, cancel := context.WithTimeout(ctx, s.postgresTimeout)
	defer cancel()
	out, err := s.repo.GetCounters(dependency, userID, ids)
	return out, dependencyError(ctx, err)
}

// The dependency budget is separate from the overall server deadline.
func dependencyError(parent context.Context, err error) error {
	if err == nil {
		return nil
	}
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return domain.ErrUnavailable
	}
	return err
}
