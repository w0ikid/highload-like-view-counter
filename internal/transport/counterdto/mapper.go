package counterdto

import (
	pb "highload-counters/api/gen/counters/v1"
	"highload-counters/internal/domain"
)

func Events(in []*pb.ViewEvent) ([]domain.ViewEvent, error) {
	out := make([]domain.ViewEvent, len(in))
	for i, e := range in {
		if e == nil {
			return nil, domain.ErrViewEventInvalid
		}
		out[i] = domain.ViewEvent{PostID: e.PostId, ViewerID: e.ViewerId}
	}
	return out, nil
}
func Counters(in []domain.Counter) *pb.GetCountersResponse {
	out := &pb.GetCountersResponse{Counters: make([]*pb.Counter, len(in))}
	for i, c := range in {
		out.Counters[i] = &pb.Counter{PostId: c.PostID, Likes: c.Likes, Views: c.Views, LikedByMe: c.LikedByMe}
	}
	return out
}
