package worker

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	"highload-counters/internal/domain"
	"log/slog"
	"time"
)

type BatchStore interface {
	Claim(context.Context) (domain.FlushBatch, error)
	Ack(context.Context, string) error
}
type Repository interface {
	ApplyBatch(context.Context, domain.FlushBatch) error
	Reconcile(context.Context) error
}
type Worker struct {
	DB                                                                      *sqlx.DB
	Store                                                                   BatchStore
	Repo                                                                    Repository
	FlushInterval, ReconcileInterval, Budget, PostgresTimeout, RedisTimeout time.Duration
}

func (w *Worker) Flush(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, w.Budget)
	defer cancel()
	redisCtx, stop := context.WithTimeout(ctx, w.RedisTimeout)
	batch, err := w.Store.Claim(redisCtx)
	stop()
	if err != nil {
		return err
	}
	if batch.ID == "" {
		return nil
	}
	pgCtx, stop := context.WithTimeout(ctx, w.PostgresTimeout)
	err = w.Repo.ApplyBatch(pgCtx, batch)
	stop()
	if err != nil {
		return err
	}
	redisCtx, stop = context.WithTimeout(ctx, w.RedisTimeout)
	defer stop()
	return w.Store.Ack(redisCtx, batch.ID)
}
func (w *Worker) Reconcile(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, w.Budget)
	defer cancel()
	return w.Repo.Reconcile(ctx)
}

// Run holds a PostgreSQL session advisory lock on a dedicated connection.
// A second worker exits; API processes never schedule background work.
func (w *Worker) Run(ctx context.Context) error {
	acquire, cancel := context.WithTimeout(ctx, w.PostgresTimeout)
	conn, err := w.DB.Connx(acquire)
	if err != nil {
		cancel()
		return err
	}
	defer conn.Close()
	var locked bool
	err = conn.QueryRowContext(acquire, `SELECT pg_try_advisory_lock(743270021)`).Scan(&locked)
	cancel()
	if err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("another counters worker owns the lock")
	}
	defer func() {
		c, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_, _ = conn.ExecContext(c, `SELECT pg_advisory_unlock(743270021)`)
	}()
	flush := time.NewTicker(w.FlushInterval)
	defer flush.Stop()
	reconcile := time.NewTicker(w.ReconcileInterval)
	defer reconcile.Stop()
	for {
		var task func(context.Context) error
		select {
		case <-ctx.Done():
			return nil
		case <-flush.C:
			task = w.Flush
		case <-reconcile.C:
			task = w.Reconcile
		}
		check, stop := context.WithTimeout(ctx, w.PostgresTimeout)
		err = conn.PingContext(check)
		stop()
		if err != nil {
			return fmt.Errorf("worker lock connection lost: %w", err)
		}
		if err = task(ctx); err != nil {
			slog.ErrorContext(ctx, "worker task failed; batch retained", "error", err)
		}
	}
}
