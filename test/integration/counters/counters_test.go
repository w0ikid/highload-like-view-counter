package counters_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "highload-counters/api/gen/counters/v1"
	"highload-counters/internal/domain"
	"highload-counters/internal/infrastructure/postgres"
	repo "highload-counters/internal/infrastructure/postgres/counter"
	redisstore "highload-counters/internal/infrastructure/redis"
	grpctransport "highload-counters/internal/transport/grpc"
	"highload-counters/internal/transport/identity"
	rest "highload-counters/internal/transport/rest/counters"
	"highload-counters/internal/usecase/counters"
	"highload-counters/internal/worker"
	"highload-counters/migrations"
	"highload-counters/pkg/counterclient"
)

type fixture struct {
	db             *sqlx.DB
	redis          *goredis.Client
	repo           *repo.Repository
	views          *redisstore.Views
	service        *counters.Service
	worker         *worker.Worker
	rest, grpc     *counterclient.Client
	dsn, namespace string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	addr := os.Getenv("TEST_REDIS_ADDR")
	if dsn == "" || addr == "" {
		t.Skip("set TEST_POSTGRES_DSN and TEST_REDIS_ADDR")
	}
	ctx := context.Background()
	admin, err := postgres.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "counters_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE"); _ = admin.Close() })
	parsed, _ := url.Parse(dsn)
	q := parsed.Query()
	q.Set("search_path", schema)
	parsed.RawQuery = q.Encode()
	db, err := postgres.Open(parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	goose.SetBaseFS(migrations.FS)
	_ = goose.SetDialect("postgres")
	if err = goose.Up(db.DB, "."); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO posts SELECT n,'Post '||n FROM generate_series(1,100) n; INSERT INTO post_counters(post_id) SELECT id FROM posts`)
	if err != nil {
		t.Fatal(err)
	}
	redis := redisstore.New(addr, time.Second)
	t.Cleanup(func() { _ = redis.Close() })
	if err = redis.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var cursor uint64
		for {
			keys, next, _ := redis.Scan(ctx, cursor, schema+":*", 100).Result()
			if len(keys) > 0 {
				_ = redis.Unlink(ctx, keys...).Err()
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	})
	repository := repo.New(db)
	views := redisstore.NewViews(redis, schema, time.Hour)
	service := counters.New(repository, views, 5*time.Second, time.Second)
	w := &worker.Worker{DB: db, Repo: repository, Store: views, FlushInterval: time.Second, ReconcileInterval: time.Hour, Budget: 5 * time.Second, PostgresTimeout: 5 * time.Second, RedisTimeout: time.Second}
	resolver := identity.Resolver{Mode: "test"}
	mux := http.NewServeMux()
	(&rest.Handler{Service: service, Identity: resolver, Timeout: 5 * time.Second}).RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpctransport.New(service, resolver, 5*time.Second)
	go grpcServer.Serve(listener)
	t.Cleanup(grpcServer.Stop)
	rc, _ := counterclient.New("rest", server.URL, 6*time.Second, 0)
	gc, _ := counterclient.New("grpc", listener.Addr().String(), 6*time.Second, 0)
	t.Cleanup(rc.Close)
	t.Cleanup(gc.Close)
	return &fixture{db, redis, repository, views, service, w, rc, gc, parsed.String(), schema}
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func (f *fixture) assertLikes(t *testing.T, post, expected int64) {
	t.Helper()
	var stored, actual int64
	must(t, f.db.QueryRow(`SELECT likes_count,(SELECT count(*) FROM likes WHERE post_id=$1) FROM post_counters WHERE post_id=$1`, post).Scan(&stored, &actual))
	if stored != expected || actual != expected {
		t.Fatalf("likes stored=%d actual=%d expected=%d", stored, actual, expected)
	}
}
func (f *fixture) viewsCount(t *testing.T, post int64) int64 {
	t.Helper()
	var n int64
	must(t, f.db.Get(&n, `SELECT views_count FROM post_counters WHERE post_id=$1`, post))
	return n
}

func TestCountersSIS2(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	t.Run("100 concurrent likes and repeated unlike", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				client := f.rest
				if i%2 == 0 {
					client = f.grpc
				}
				_, err := client.Like(ctx, 1, 1, true)
				if err != nil {
					t.Error(err)
				}
			}(i)
		}
		wg.Wait()
		f.assertLikes(t, 1, 1)
		for i := 0; i < 3; i++ {
			_, err := f.rest.Like(ctx, 1, 1, false)
			must(t, err)
		}
		f.assertLikes(t, 1, 0)
	})
	t.Run("cross protocol dedup and full validation", func(t *testing.T) {
		events := []*pb.ViewEvent{{PostId: 2, ViewerId: 200}}
		first, err := f.rest.Views(ctx, 1, events)
		must(t, err)
		if first.Accepted != 1 {
			t.Fatal(first)
		}
		again, err := f.grpc.Views(ctx, 1, events)
		must(t, err)
		if again.Duplicates != 1 {
			t.Fatal(again)
		}
		for _, client := range []*counterclient.Client{f.rest, f.grpc} {
			_, err = client.Views(ctx, 1, []*pb.ViewEvent{{PostId: 3, ViewerId: 300}, {PostId: 999, ViewerId: 300}})
			if err == nil {
				t.Fatal("accepted unknown post")
			}
			_, err = client.Views(ctx, 1, []*pb.ViewEvent{{PostId: 3, ViewerId: 300}, {PostId: 4, ViewerId: 0}})
			if err == nil {
				t.Fatal("accepted invalid event")
			}
			_, err = client.Counters(ctx, 1, []int64{1, 1})
			if err == nil {
				t.Fatal("accepted repeated post IDs")
			}
			_, err = client.Like(ctx, 0, 1, true)
			if err == nil {
				t.Fatal("accepted missing identity")
			}
		}
		must(t, f.worker.Flush(ctx))
		if f.viewsCount(t, 2) != 1 || f.viewsCount(t, 3) != 0 {
			t.Fatal("partial validation write or incorrect dedup")
		}
	})
	t.Run("dedup window does not slide", func(t *testing.T) {
		store := redisstore.NewViews(f.redis, f.namespace+":ttl", 400*time.Millisecond)
		event := []domain.ViewEvent{{PostID: 4, ViewerID: 400}}
		out, err := store.Record(ctx, event)
		must(t, err)
		if out.Accepted != 1 {
			t.Fatal(out)
		}
		time.Sleep(250 * time.Millisecond)
		out, err = store.Record(ctx, event)
		must(t, err)
		if out.Duplicates != 1 {
			t.Fatal(out)
		}
		time.Sleep(200 * time.Millisecond)
		out, err = store.Record(ctx, event)
		must(t, err)
		if out.Accepted != 1 {
			t.Fatal("duplicate extended TTL")
		}
		batch, err := store.Claim(ctx)
		must(t, err)
		if len(batch.Deltas) != 1 || batch.Deltas[0].Delta != 2 {
			t.Fatal(batch)
		}
		must(t, store.Ack(ctx, batch.ID))
	})
	t.Run("concurrent mixed likes and reconciler", func(t *testing.T) {
		_, err := f.db.Exec(`UPDATE post_counters SET likes_count=42 WHERE post_id=5`)
		must(t, err)
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				for j := 0; j < 25; j++ {
					if err := f.repo.SetLike(ctx, int64(i+1), 5, j%2 == 0); err != nil {
						t.Error(err)
					}
				}
			}(i)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 4; i++ {
				if err := f.repo.Reconcile(ctx); err != nil {
					t.Error(err)
				}
			}
		}()
		wg.Wait()
		f.assertLikes(t, 5, 8)
	})
	t.Run("worker process crash before and after commit", func(t *testing.T) {
		for _, stage := range []string{"before", "after"} {
			before := f.viewsCount(t, 6)
			_, err := f.views.Record(ctx, []domain.ViewEvent{{PostID: 6, ViewerID: time.Now().UnixNano()}})
			must(t, err)
			command := exec.Command(os.Args[0], "-test.run=^TestCrashHelper$")
			command.Env = append(os.Environ(), "COUNTER_CRASH_STAGE="+stage, "COUNTER_CRASH_DSN="+f.dsn, "COUNTER_CRASH_NAMESPACE="+f.namespace)
			err = command.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 99 {
				t.Fatalf("helper exit: %v", err)
			}
			// New store instance simulates restarted worker, no process-local batch state.
			f.worker.Store = redisstore.NewViews(f.redis, f.namespace, time.Hour)
			// New arrivals during in-flight must survive its acknowledgment.
			_, err = f.views.Record(ctx, []domain.ViewEvent{{PostID: 6, ViewerID: time.Now().UnixNano()}})
			must(t, err)
			must(t, f.worker.Flush(ctx))
			must(t, f.worker.Flush(ctx))
			must(t, f.worker.Flush(ctx))
			if f.viewsCount(t, 6) != before+2 {
				t.Fatal("lost or double-applied views")
			}
		}
	})
	t.Run("dependency outage preserves batch and likes", func(t *testing.T) {
		badRedis := redisstore.New("127.0.0.1:1", 20*time.Millisecond)
		defer badRedis.Close()
		service := counters.New(f.repo, redisstore.NewViews(badRedis, "down", time.Hour), time.Second, 50*time.Millisecond)
		_, err := service.RecordViews(ctx, 1, []domain.ViewEvent{{PostID: 7, ViewerID: 700}})
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("Redis outage: %v", err)
		}
		must(t, service.SetLike(ctx, 1, 7, true))
		f.assertLikes(t, 7, 1)
		_, err = f.views.Record(ctx, []domain.ViewEvent{{PostID: 7, ViewerID: 700}})
		must(t, err)
		badDB, err := postgres.Open("postgres://none:none@127.0.0.1:1/none?sslmode=disable&connect_timeout=1")
		must(t, err)
		defer badDB.Close()
		broken := *f.worker
		broken.Repo = repo.New(badDB)
		if err = broken.Flush(ctx); err == nil {
			t.Fatal("flush acknowledged unavailable PostgreSQL")
		}
		if err = repo.New(badDB).SetLike(ctx, 1, 7, true); err == nil {
			t.Fatal("like acknowledged unavailable PostgreSQL")
		}
		must(t, f.worker.Flush(ctx))
		if f.viewsCount(t, 7) != 1 {
			t.Fatal("pending lost")
		}
	})
	t.Run("freshness under two seconds and one worker", func(t *testing.T) {
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- f.worker.Run(runCtx) }()
		defer func() { cancel(); must(t, <-done) }()
		time.Sleep(100 * time.Millisecond)
		_, err := f.rest.Views(ctx, 1, []*pb.ViewEvent{{PostId: 8, ViewerId: 800}})
		must(t, err)
		start := time.Now()
		for {
			result, err := f.grpc.Counters(ctx, 1, []int64{8})
			must(t, err)
			if result.Counters[0].Views == 1 {
				break
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("freshness exceeded 2s")
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Logf("accepted-to-visible latency: %s", time.Since(start))
		other := *f.worker
		otherCtx, stop := context.WithTimeout(ctx, time.Second)
		defer stop()
		if err = other.Run(otherCtx); err == nil || !strings.Contains(err.Error(), "another counters worker") {
			t.Fatalf("duplicate worker: %v", err)
		}
	})
	t.Run("retry after accepted response failure keeps event identity", func(t *testing.T) {
		mux := http.NewServeMux()
		(&rest.Handler{Service: f.service, Identity: identity.Resolver{Mode: "test"}, Timeout: time.Second}).RegisterRoutes(mux)
		var calls atomic.Int32
		flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, r)
				w.WriteHeader(503)
				return
			}
			mux.ServeHTTP(w, r)
		}))
		defer flaky.Close()
		client, err := counterclient.New("rest", flaky.URL, 2*time.Second, 2)
		must(t, err)
		defer client.Close()
		result, err := client.Views(ctx, 1, []*pb.ViewEvent{{PostId: 10, ViewerId: 1000}})
		must(t, err)
		if result.Duplicates != 1 || calls.Load() != 2 {
			t.Fatalf("retry: %+v calls=%d", result, calls.Load())
		}
		must(t, f.worker.Flush(ctx))
		if f.viewsCount(t, 10) != 1 {
			t.Fatal("retry doubled effect")
		}
	})
	t.Run("server deadline maps equally across protocols", func(t *testing.T) {
		tx, err := f.db.BeginTxx(ctx, nil)
		must(t, err)
		defer tx.Rollback()
		_, err = tx.Exec(`SELECT post_id FROM post_counters WHERE post_id=11 FOR UPDATE`)
		must(t, err)
		resolver := identity.Resolver{Mode: "test"}
		mux := http.NewServeMux()
		(&rest.Handler{Service: f.service, Identity: resolver, Timeout: 40 * time.Millisecond}).RegisterRoutes(mux)
		server := httptest.NewServer(mux)
		defer server.Close()
		rc, err := counterclient.New("rest", server.URL, time.Second, 0)
		must(t, err)
		defer rc.Close()
		_, err = rc.Like(ctx, 1, 11, true)
		var httpErr *counterclient.HTTPError
		if !errors.As(err, &httpErr) || httpErr.Status != 504 {
			t.Fatalf("REST server deadline: %v", err)
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		must(t, err)
		gs := grpctransport.New(f.service, resolver, 40*time.Millisecond)
		go gs.Serve(listener)
		defer gs.Stop()
		gc, err := counterclient.New("grpc", listener.Addr().String(), time.Second, 0)
		must(t, err)
		defer gc.Close()
		_, err = gc.Like(ctx, 1, 11, true)
		if status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("gRPC server deadline: %v", err)
		}
	})
	t.Run("deadline cancellation and codes", func(t *testing.T) {
		_, err := f.grpc.Like(ctx, 1, 999, true)
		if status.Code(err) != codes.NotFound {
			t.Fatal(err)
		}
		_, err = f.grpc.Like(ctx, 1, -1, true)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatal(err)
		}
		tx, err := f.db.BeginTxx(ctx, nil)
		must(t, err)
		defer tx.Rollback()
		_, err = tx.Exec(`SELECT post_id FROM post_counters WHERE post_id=9 FOR UPDATE`)
		must(t, err)
		for _, client := range []*counterclient.Client{f.rest, f.grpc} {
			limited, stop := context.WithTimeout(ctx, 50*time.Millisecond)
			_, err = client.Like(limited, 1, 9, true)
			stop()
			if err == nil {
				t.Fatal("ignored client deadline")
			}
			canceled, stop := context.WithCancel(ctx)
			stop()
			_, err = client.Like(canceled, 1, 9, true)
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		}
		must(t, tx.Rollback())
		f.assertLikes(t, 9, 0)
	})
}

func TestCrashHelper(t *testing.T) {
	stage := os.Getenv("COUNTER_CRASH_STAGE")
	if stage == "" {
		t.Skip("subprocess helper")
	}
	db, err := postgres.Open(os.Getenv("COUNTER_CRASH_DSN"))
	must(t, err)
	redis := redisstore.New(os.Getenv("TEST_REDIS_ADDR"), time.Second)
	store := redisstore.NewViews(redis, os.Getenv("COUNTER_CRASH_NAMESPACE"), time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	batch, err := store.Claim(ctx)
	must(t, err)
	if batch.ID == "" {
		t.Fatal("missing batch")
	}
	if stage == "after" {
		must(t, repo.New(db).ApplyBatch(ctx, batch))
	}
	fmt.Fprintln(os.Stderr, "simulated abrupt worker death at", stage)
	os.Exit(99)
}
