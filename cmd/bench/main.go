package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	pb "highload-counters/api/gen/counters/v1"
	"highload-counters/pkg/counterclient"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	protocol := flag.String("protocol", "rest", "rest or grpc")
	address := flag.String("address", "http://localhost:8080", "API address")
	op := flag.String("op", "read", "read, views, likes, duplicates, hot, freshness")
	concurrency := flag.Int("concurrency", 1, "concurrent clients")
	duration := flag.Duration("duration", 60*time.Second, "measurement duration")
	warmup := flag.Duration("warmup", 10*time.Second, "warmup duration")
	budget := flag.Duration("budget", 2*time.Second, "total per-call budget")
	retries := flag.Int("retries", 0, "additional attempts: 0..2; leave zero for protocol comparison")
	payload := flag.Bool("payload", false, "print JSON and Protobuf body sizes only")
	flag.Parse()
	if *payload {
		return payloads()
	}
	if *concurrency < 1 || *duration <= 0 || *warmup < 0 {
		return fmt.Errorf("invalid benchmark parameters")
	}
	switch *op {
	case "read", "views", "likes", "duplicates", "hot", "freshness":
	default:
		return fmt.Errorf("invalid operation")
	}
	client, err := counterclient.New(*protocol, *address, *budget, *retries)
	if err != nil {
		return err
	}
	defer client.Close()
	if *op == "freshness" {
		return measureFreshness(client, *concurrency, *duration)
	}
	ids := make([]int64, 20)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	var sequence atomic.Int64
	call := func(ctx context.Context) error {
		n := sequence.Add(1)
		switch *op {
		case "read":
			_, err := client.Counters(ctx, 1, ids)
			return err
		case "views", "duplicates":
			events := make([]*pb.ViewEvent, 50)
			for i := range events {
				viewer := n*50 + int64(i) + 1
				if *op == "duplicates" {
					viewer = int64(i + 1)
				}
				events[i] = &pb.ViewEvent{PostId: int64(i%20 + 1), ViewerId: viewer}
			}
			_, err := client.Views(ctx, 1, events)
			return err
		default:
			post := (n/2)%100 + 1
			if *op == "hot" {
				post = 1
			}
			_, err := client.Like(ctx, n/2+1, post, n%2 == 1)
			return err
		}
	}
	type sample struct {
		Microseconds float64 `json:"us"`
		Error        string  `json:"error,omitempty"`
	}
	phase := func(length time.Duration, record bool) ([]sample, time.Duration) {
		if length == 0 {
			return nil, 0
		}
		start := time.Now()
		deadline := start.Add(length)
		var wg sync.WaitGroup
		var mu sync.Mutex
		var all []sample
		for i := 0; i < *concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var local []sample
				for time.Now().Before(deadline) {
					before := time.Now()
					err := call(context.Background())
					if record {
						s := sample{Microseconds: float64(time.Since(before).Nanoseconds()) / 1000}
						if err != nil {
							s.Error = err.Error()
						}
						local = append(local, s)
					}
				}
				mu.Lock()
				all = append(all, local...)
				mu.Unlock()
			}()
		}
		wg.Wait()
		return all, time.Since(start)
	}
	phase(*warmup, false)
	samples, elapsed := phase(*duration, true)
	latencies := make([]float64, len(samples))
	success := 0
	errorCounts := map[string]int{}
	for i, s := range samples {
		latencies[i] = s.Microseconds
		if s.Error == "" {
			success++
		} else {
			errorCounts[s.Error]++
		}
	}
	sort.Float64s(latencies)
	percentile := func(p float64) float64 {
		if len(latencies) == 0 {
			return 0
		}
		return latencies[int(float64(len(latencies)-1)*p)] / 1000
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"protocol": *protocol, "operation": *op, "concurrency": *concurrency, "warmup": warmup.String(), "requestedDuration": duration.String(), "elapsedSeconds": elapsed.Seconds(), "retries": *retries,
		"success": success, "errors": len(samples) - success, "errorCounts": errorCounts, "rps": float64(len(samples)) / elapsed.Seconds(), "successfulRps": float64(success) / elapsed.Seconds(),
		"p50ms": percentile(.50), "p95ms": percentile(.95), "p99ms": percentile(.99), "go": runtime.Version(), "latencyPopulation": "all attempts including failed requests; includes in-flight drain", "samples": samples,
	})
}
func payloads() error {
	ids := make([]int64, 20)
	counters := make([]*pb.Counter, 20)
	for i := range ids {
		ids[i] = int64(i + 1)
		counters[i] = &pb.Counter{PostId: ids[i], Likes: 123, Views: 456, LikedByMe: i%2 == 0}
	}
	events := make([]*pb.ViewEvent, 50)
	for i := range events {
		events[i] = &pb.ViewEvent{PostId: int64(i%20 + 1), ViewerId: int64(i + 1000)}
	}
	pairs := []struct {
		Name              string
		Request, Response proto.Message
	}{
		{"read20", &pb.GetCountersRequest{PostIds: ids}, &pb.GetCountersResponse{Counters: counters}},
		{"views50", &pb.RecordViewsRequest{Events: events}, &pb.RecordViewsResponse{Accepted: 50}},
		{"like", &pb.LikeRequest{PostId: 1}, &pb.LikeResponse{PostId: 1, Liked: true}},
	}
	var out []map[string]any
	for _, pair := range pairs {
		q, _ := (protojson.MarshalOptions{EmitUnpopulated: true}).Marshal(pair.Request)
		a, _ := (protojson.MarshalOptions{EmitUnpopulated: true}).Marshal(pair.Response)
		out = append(out, map[string]any{"operation": pair.Name, "jsonRequest": len(q), "protobufRequest": proto.Size(pair.Request), "jsonResponse": len(a), "protobufResponse": proto.Size(pair.Response)})
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"note": "Logical serialized bodies only. REST GET and path-based like actually have no request body; headers, URLs, gRPC framing and TLS excluded. JSON int64 uses strings.", "sizes": out})
}

// Freshness probes post 100 while background writers submit 50-event batches
// to posts 1..20. Each probe waits for its own monotonic persisted count.
func measureFreshness(client *counterclient.Client, concurrency int, duration time.Duration) error {
	initial, err := client.Counters(context.Background(), 1, []int64{100})
	if err != nil {
		return err
	}
	expected := initial.Counters[0].Views
	end := time.Now().Add(duration)
	start := time.Now()
	var sequence, success, failed atomic.Int64
	var wg sync.WaitGroup
	seed := time.Now().UnixNano() / 1000
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(end) {
				n := sequence.Add(1)
				events := make([]*pb.ViewEvent, 50)
				for j := range events {
					events[j] = &pb.ViewEvent{PostId: int64(j%20 + 1), ViewerId: seed + n*50 + int64(j)}
				}
				if _, err := client.Views(context.Background(), 1, events); err != nil {
					failed.Add(1)
				} else {
					success.Add(1)
				}
			}
		}()
	}
	var delays []float64
	violations := 0
	for time.Now().Before(end) {
		_, err := client.Views(context.Background(), 1, []*pb.ViewEvent{{PostId: 100, ViewerId: time.Now().UnixNano()}})
		if err != nil {
			failed.Add(1)
			continue
		}
		expected++
		accepted := time.Now()
		visible := false
		for time.Since(accepted) < 2*time.Second {
			counters, err := client.Counters(context.Background(), 1, []int64{100})
			if err == nil && counters.Counters[0].Views >= expected {
				visible = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		elapsed := float64(time.Since(accepted).Nanoseconds()) / 1e6
		delays = append(delays, elapsed)
		if !visible || elapsed > 2000 {
			violations++
		}
	}
	wg.Wait()
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"scenario": "freshness under concurrent views50 load", "concurrency": concurrency, "seconds": time.Since(start).Seconds(), "successfulBatches": success.Load(), "failedBatches": failed.Load(), "acceptedToVisibleMs": delays, "violationsOver2s": violations})
}
