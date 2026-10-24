package counters

import (
	"context"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	pb "highload-counters/api/gen/counters/v1"
	"highload-counters/internal/domain"
	"highload-counters/internal/transport/counterdto"
	"highload-counters/internal/transport/identity"
	"highload-counters/internal/transport/rest/response"
	"highload-counters/internal/usecase/counters"
	"highload-counters/pkg/traceid"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	Service  *counters.Service
	Identity identity.Resolver
	Timeout  time.Duration
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("PUT /v1/posts/{id}/like", h.wrap(h.like(true)))
	mux.HandleFunc("DELETE /v1/posts/{id}/like", h.wrap(h.like(false)))
	mux.HandleFunc("POST /v1/views", h.wrap(h.views))
	mux.HandleFunc("GET /v1/counters", h.wrap(h.counters))
}

type action func(context.Context, int64, *http.Request) (proto.Message, int, error)

func (h *Handler) wrap(fn action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := traceid.New()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), h.Timeout)
		defer cancel()
		r = r.WithContext(ctx)
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		user, err := h.Identity.Resolve(ctx, r.Header.Get("X-User-ID"), r.Header.Get("Authorization"))
		var body proto.Message
		var status int
		if err == nil {
			body, status, err = fn(ctx, user, r)
		}
		if err != nil {
			response.HandleError(w, r, id, err)
			return
		}
		raw, err := (protojson.MarshalOptions{EmitUnpopulated: true}).Marshal(body)
		if err != nil {
			response.HandleError(w, r, id, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(raw)
	}
}
func (h *Handler) like(liked bool) action {
	return func(ctx context.Context, user int64, r *http.Request) (proto.Message, int, error) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			return nil, 0, domain.ErrPostIDInvalid
		}
		if err = h.Service.SetLike(ctx, user, id, liked); err != nil {
			return nil, 0, err
		}
		return &pb.LikeResponse{PostId: id, Liked: liked}, http.StatusOK, nil
	}
}
func (h *Handler) views(ctx context.Context, user int64, r *http.Request) (proto.Message, int, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, 0, domain.ErrValidation
	}
	var in pb.RecordViewsRequest
	if err = protojson.Unmarshal(raw, &in); err != nil {
		return nil, 0, domain.ErrValidation
	}
	events, err := counterdto.Events(in.Events)
	if err != nil {
		return nil, 0, err
	}
	result, err := h.Service.RecordViews(ctx, user, events)
	if err != nil {
		return nil, 0, err
	}
	return &pb.RecordViewsResponse{Accepted: int32(result.Accepted), Duplicates: int32(result.Duplicates)}, http.StatusAccepted, nil
}
func (h *Handler) counters(ctx context.Context, user int64, r *http.Request) (proto.Message, int, error) {
	values := strings.Split(r.URL.Query().Get("postIds"), ",")
	if len(values) > domain.MaxCounterPosts {
		return nil, 0, domain.ErrCounterBatchInvalid
	}
	ids := make([]int64, len(values))
	for i, value := range values {
		var err error
		ids[i], err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, 0, domain.ErrCounterBatchInvalid
		}
	}
	result, err := h.Service.GetCounters(ctx, user, ids)
	if err != nil {
		return nil, 0, err
	}
	return counterdto.Counters(result), http.StatusOK, nil
}
