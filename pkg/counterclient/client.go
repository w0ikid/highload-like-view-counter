// Package counterclient shares transport operations and bounded retry policy.
package counterclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	pb "highload-counters/api/gen/counters/v1"
)

type Client struct {
	protocol, base string
	http           *http.Client
	conn           *grpc.ClientConn
	rpc            pb.CounterServiceClient
	Budget         time.Duration
	Retries        int
}

func New(protocol, address string, budget time.Duration, retries int) (*Client, error) {
	if retries < 0 || retries > 2 || budget <= 0 {
		return nil, fmt.Errorf("retries must be 0..2 and budget positive")
	}
	c := &Client{protocol: protocol, base: strings.TrimRight(address, "/"), Budget: budget, Retries: retries}
	switch protocol {
	case "rest":
		c.http = &http.Client{Transport: &http.Transport{MaxIdleConns: 100, MaxIdleConnsPerHost: 100, MaxConnsPerHost: 100, DisableCompression: true, ForceAttemptHTTP2: false}}
	case "grpc":
		conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
		if err != nil {
			return nil, err
		}
		c.conn = conn
		c.rpc = pb.NewCounterServiceClient(conn)
	default:
		return nil, fmt.Errorf("protocol must be rest or grpc")
	}
	return c, nil
}
func (c *Client) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	if c.http != nil {
		c.http.CloseIdleConnections()
	}
}

type HTTPError struct{ Status int }

func (e *HTTPError) Error() string { return fmt.Sprintf("HTTP %d", e.Status) }
func temporary(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Status == 503
	}
	if status.Code(err) == codes.Unavailable {
		return true
	}
	var network net.Error
	return errors.As(err, &network) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// Retry uses one total deadline, at most two additional attempts, and full
// operation identity supplied by the caller. No lower-level gRPC retries.
func Retry(ctx context.Context, budget time.Duration, retries int, fn func(context.Context) error) error {
	if retries < 0 || retries > 2 {
		return fmt.Errorf("retries must be 0..2")
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt >= retries || !temporary(err) {
			return err
		}
		base := 100 * time.Millisecond * time.Duration(1<<attempt)
		timer := time.NewTimer(base/2 + time.Duration(rand.Int64N(int64(base/2))))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func (c *Client) rest(ctx context.Context, user int64, method, path string, in, out proto.Message) error {
	var body []byte
	var err error
	if in != nil {
		body, err = (protojson.MarshalOptions{EmitUnpopulated: true}).Marshal(in)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-User-ID", strconv.FormatInt(user, 10))
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &HTTPError{res.StatusCode}
	}
	return protojson.Unmarshal(raw, out)
}
func rpcContext(ctx context.Context, user int64) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-user-id", strconv.FormatInt(user, 10))
}
func (c *Client) Like(ctx context.Context, user, post int64, liked bool) (*pb.LikeResponse, error) {
	var out *pb.LikeResponse
	err := Retry(ctx, c.Budget, c.Retries, func(ctx context.Context) error {
		var err error
		in := &pb.LikeRequest{PostId: post}
		if c.protocol == "grpc" {
			if liked {
				out, err = c.rpc.LikePost(rpcContext(ctx, user), in)
			} else {
				out, err = c.rpc.UnlikePost(rpcContext(ctx, user), in)
			}
			return err
		}
		method := "DELETE"
		if liked {
			method = "PUT"
		}
		out = &pb.LikeResponse{}
		return c.rest(ctx, user, method, fmt.Sprintf("/v1/posts/%d/like", post), nil, out)
	})
	return out, err
}
func (c *Client) Views(ctx context.Context, user int64, events []*pb.ViewEvent) (*pb.RecordViewsResponse, error) {
	var out *pb.RecordViewsResponse
	err := Retry(ctx, c.Budget, c.Retries, func(ctx context.Context) error {
		in := &pb.RecordViewsRequest{Events: events}
		if c.protocol == "grpc" {
			var err error
			out, err = c.rpc.RecordViews(rpcContext(ctx, user), in)
			return err
		}
		out = &pb.RecordViewsResponse{}
		return c.rest(ctx, user, "POST", "/v1/views", in, out)
	})
	return out, err
}
func (c *Client) Counters(ctx context.Context, user int64, ids []int64) (*pb.GetCountersResponse, error) {
	var out *pb.GetCountersResponse
	err := Retry(ctx, c.Budget, c.Retries, func(ctx context.Context) error {
		if c.protocol == "grpc" {
			var err error
			out, err = c.rpc.GetCounters(rpcContext(ctx, user), &pb.GetCountersRequest{PostIds: ids})
			return err
		}
		values := make([]string, len(ids))
		for i, id := range ids {
			values[i] = strconv.FormatInt(id, 10)
		}
		out = &pb.GetCountersResponse{}
		return c.rest(ctx, user, "GET", "/v1/counters?postIds="+strings.Join(values, ","), nil, out)
	})
	return out, err
}
