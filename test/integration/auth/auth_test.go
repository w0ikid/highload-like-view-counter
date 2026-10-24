package auth_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	"highload-counters/internal/domain"
	"highload-counters/internal/infrastructure/postgres"
	"highload-counters/internal/infrastructure/postgres/credential"
	"highload-counters/internal/infrastructure/postgres/passwordreset"
	"highload-counters/internal/infrastructure/postgres/session"
	"highload-counters/internal/infrastructure/postgres/user"
	"highload-counters/internal/infrastructure/sqltx"
	"highload-counters/internal/infrastructure/token"
	"highload-counters/internal/usecase/auth"
	"highload-counters/migrations"
)

type mailbox struct {
	token string
	count int
}

func (m *mailbox) SendPasswordReset(_ context.Context, _ string, token string) error {
	m.token = token
	m.count++
	return nil
}

type failingSessions struct{ domain.SessionRepository }

func (f failingSessions) Create(context.Context, domain.Session) error { return domain.ErrUnavailable }

func TestAuthLifecycle(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := postgres.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "auth_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, _ = admin.ExecContext(cleanup, "DROP SCHEMA "+schema+" CASCADE")
	}()
	uri, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := uri.Query()
	query.Set("search_path", schema)
	uri.RawQuery = query.Encode()
	db, err := postgres.Open(uri.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.FS)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, db.DB, "."); err != nil {
		t.Fatal(err)
	}
	box := &mailbox{}
	svc := auth.New(user.New(db), credential.New(db), session.New(db), passwordreset.New(db), sqltx.NewManager(db), token.New("test-secret-at-least-thirty-two-bytes", time.Minute), box, auth.Config{RefreshTTL: time.Hour, ResetTTL: time.Minute, DBTimeout: 5 * time.Second})
	first, err := svc.Register(ctx, " Test@Example.com ", "correct-password", "Test User")
	if err != nil {
		t.Fatal(err)
	}
	me, err := svc.Me(ctx, first.AccessToken)
	if err != nil || me.Email != "test@example.com" {
		t.Fatalf("me: %+v %v", me, err)
	}
	if _, err = svc.Register(ctx, "test@example.com", "correct-password", "Duplicate"); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate email: %v", err)
	}
	if _, err = svc.Login(ctx, "test@example.com", "wrong-password"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err = svc.Login(ctx, "missing@example.com", "wrong-password"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("unknown email: %v", err)
	}
	var wins atomic.Int64
	var wg sync.WaitGroup
	var rotated domain.TokenPair
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pair, err := svc.Refresh(ctx, first.RefreshToken)
			if err == nil {
				wins.Add(1)
				rotated = pair
			} else if !errors.Is(err, domain.ErrUnauthorized) {
				t.Errorf("refresh: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("refresh succeeded %d times", wins.Load())
	}
	if _, err = svc.Me(ctx, first.AccessToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("old access after rotation: %v", err)
	}
	if _, err = svc.Me(ctx, rotated.AccessToken); err != nil {
		t.Fatal(err)
	}
	if err = svc.ForgotPassword(ctx, "missing@example.com"); err != nil {
		t.Fatal(err)
	}
	if box.count != 0 {
		t.Fatal("mail sent for unknown account")
	}
	if err = svc.ForgotPassword(ctx, "test@example.com"); err != nil {
		t.Fatal(err)
	}
	oldReset := box.token
	if err = svc.ForgotPassword(ctx, "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err = svc.ResetPassword(ctx, oldReset, "new-password"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("old reset: %v", err)
	}
	wins.Store(0)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.ResetPassword(ctx, box.token, "new-password")
			if err == nil {
				wins.Add(1)
			} else if !errors.Is(err, domain.ErrUnauthorized) {
				t.Errorf("reset: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("reset succeeded %d times", wins.Load())
	}
	if _, err = svc.Me(ctx, rotated.AccessToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("reset left access valid: %v", err)
	}
	if _, err = svc.Refresh(ctx, rotated.RefreshToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("reset left refresh valid: %v", err)
	}
	if _, err = svc.Login(ctx, "test@example.com", "correct-password"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("old password: %v", err)
	}
	fresh, err := svc.Login(ctx, "test@example.com", "new-password")
	if err != nil {
		t.Fatal(err)
	}
	// A failure after revoking the old refresh must roll back that revocation.
	broken := auth.New(user.New(db), credential.New(db), failingSessions{session.New(db)}, passwordreset.New(db), sqltx.NewManager(db), token.New("test-secret-at-least-thirty-two-bytes", time.Minute), box, auth.Config{RefreshTTL: time.Hour, ResetTTL: time.Minute, DBTimeout: 5 * time.Second})
	if _, err = broken.Refresh(ctx, fresh.RefreshToken); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("expected session insertion failure: %v", err)
	}
	if _, err = svc.Me(ctx, fresh.AccessToken); err != nil {
		t.Fatalf("failed rotation revoked original session: %v", err)
	}
	if err = svc.Logout(ctx, fresh.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if err = svc.Logout(ctx, fresh.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Me(ctx, fresh.AccessToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("logout left access valid: %v", err)
	}
	if err = svc.ForgotPassword(ctx, "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE password_resets SET expires_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if err = svc.ResetPassword(ctx, box.token, "another-password"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired reset: %v", err)
	}
	var hash string
	if err = db.GetContext(ctx, &hash, `SELECT password_hash FROM user_credentials WHERE user_id=$1`, me.ID); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("unexpected password format")
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err = svc.Register(canceled, "cancel@example.com", "valid-password", "Canceled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	// All refresh and reset rows contain hashes, never raw credentials.
	var n int
	if err = db.GetContext(ctx, &n, `SELECT count(*) FROM sessions WHERE length(refresh_token_hash)<>64`); err != nil || n != 0 {
		t.Fatal(fmt.Sprintf("invalid hashes: %d %v", n, err))
	}
}
