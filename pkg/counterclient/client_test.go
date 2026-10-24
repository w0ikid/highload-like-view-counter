package counterclient

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBoundedRetries(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), time.Second, 2, func(context.Context) error { calls++; return &HTTPError{503} })
	if err == nil || calls != 3 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	for _, code := range []int{400, 401, 404} {
		calls = 0
		_ = Retry(context.Background(), time.Second, 2, func(context.Context) error { calls++; return &HTTPError{code} })
		if calls != 1 {
			t.Fatalf("retried HTTP %d", code)
		}
	}
	calls = 0
	start := time.Now()
	err = Retry(context.Background(), 20*time.Millisecond, 2, func(context.Context) error { calls++; return &HTTPError{503} })
	if !errors.Is(err, context.DeadlineExceeded) || calls != 1 || time.Since(start) > 100*time.Millisecond {
		t.Fatalf("budget: calls=%d elapsed=%s err=%v", calls, time.Since(start), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	err = Retry(ctx, time.Second, 2, func(context.Context) error { calls++; return nil })
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled client attempted work")
	}
}
