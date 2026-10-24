package main

import (
	"context"
	"fmt"
	"github.com/pressly/goose/v3"
	"highload-counters/internal/infrastructure/postgres"
	"highload-counters/migrations"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: migrate <up|down|status|version>")
	}
	command := os.Args[1]
	switch command {
	case "up", "down", "status", "version":
	default:
		return fmt.Errorf("unsupported migration command")
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
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.RunContext(ctx, command, db.DB, ".")
}
