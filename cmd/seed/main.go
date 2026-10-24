package main

import (
	"context"
	"flag"
	"fmt"
	"highload-counters/internal/infrastructure/postgres"
	"highload-counters/internal/infrastructure/sqltx"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}
func run() error {
	count := flag.Int("posts", 100, "number of test posts (1..100000)")
	flag.Parse()
	if *count < 1 || *count > 100000 {
		return fmt.Errorf("posts must be in 1..100000")
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		return fmt.Errorf("POSTGRES_DSN is required")
	}
	db, err := postgres.Open(dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	return sqltx.NewManager(db).WithinTx(ctx, func(ctx context.Context) error {
		tx := sqltx.From(ctx)
		if _, err := tx.ExecContext(ctx, `INSERT INTO posts (id,title) SELECT n, 'Test post ' || n FROM generate_series(1,$1::bigint) n ON CONFLICT DO NOTHING`, *count); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO post_counters (post_id) SELECT id FROM posts ON CONFLICT DO NOTHING`)
		return err
	})
}
