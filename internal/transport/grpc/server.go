package grpc

import (
	"context"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	pb "highload-counters/api/gen/counters/v1"
	"highload-counters/internal/domain"
	"highload-counters/internal/transport/counterdto"
	"highload-counters/internal/transport/identity"
	"highload-counters/internal/usecase/counters"
	"highload-counters/pkg/traceid"
	"time"
)

type key struct{}
type Server struct {
	pb.UnimplementedCounterServiceServer
	Service *counters.Service
}

func New(service *counters.Service, resolver identity.Resolver, budget time.Duration) *grpc.Server {
	server := grpc.NewServer(grpc.MaxRecvMsgSize(16384), grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, cancel := context.WithTimeout(ctx, budget)
		defer cancel()
		_ = grpc.SetHeader(ctx, metadata.Pairs("x-request-id", traceid.New()))
		md, _ := metadata.FromIncomingContext(ctx)
		first := func(name string) string {
			values := md.Get(name)
			if len(values) != 1 {
				return ""
			}
			return values[0]
		}
		id, err := resolver.Resolve(ctx, first("x-user-id"), first("authorization"))
		if err != nil {
			return nil, mapError(err)
		}
		out, err := handler(context.WithValue(ctx, key{}, id), req)
		return out, mapError(err)
	}))
	pb.RegisterCounterServiceServer(server, &Server{Service: service})
	return server
}
func (s *Server) LikePost(ctx context.Context, in *pb.LikeRequest) (*pb.LikeResponse, error) {
	return s.like(ctx, in, true)
}
func (s *Server) UnlikePost(ctx context.Context, in *pb.LikeRequest) (*pb.LikeResponse, error) {
	return s.like(ctx, in, false)
}
func (s *Server) like(ctx context.Context, in *pb.LikeRequest, liked bool) (*pb.LikeResponse, error) {
	if err := s.Service.SetLike(ctx, ctx.Value(key{}).(int64), in.PostId, liked); err != nil {
		return nil, err
	}
	return &pb.LikeResponse{PostId: in.PostId, Liked: liked}, nil
}
func (s *Server) RecordViews(ctx context.Context, in *pb.RecordViewsRequest) (*pb.RecordViewsResponse, error) {
	events, err := counterdto.Events(in.Events)
	if err != nil {
		return nil, err
	}
	out, err := s.Service.RecordViews(ctx, ctx.Value(key{}).(int64), events)
	if err != nil {
		return nil, err
	}
	return &pb.RecordViewsResponse{Accepted: int32(out.Accepted), Duplicates: int32(out.Duplicates)}, nil
}
func (s *Server) GetCounters(ctx context.Context, in *pb.GetCountersRequest) (*pb.GetCountersResponse, error) {
	out, err := s.Service.GetCounters(ctx, ctx.Value(key{}).(int64), in.PostIds)
	if err != nil {
		return nil, err
	}
	return counterdto.Counters(out), nil
}
func mapError(err error) error {
	if err == nil {
		return nil
	}
	code := codes.Internal
	message := "internal error"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "request deadline exceeded")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request canceled")
	case errors.Is(err, domain.ErrValidation):
		code = codes.InvalidArgument
	case errors.Is(err, domain.ErrUnauthorized):
		code = codes.Unauthenticated
	case errors.Is(err, domain.ErrNotFound):
		code = codes.NotFound
	case errors.Is(err, domain.ErrUnavailable):
		code = codes.Unavailable
	}
	if code != codes.Internal {
		if de, ok := domain.AsError(err); ok {
			message = de.Message
		}
	}
	return status.Error(code, message)
}
