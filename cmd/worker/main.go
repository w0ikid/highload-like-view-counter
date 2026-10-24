package main

import (
	"context"
	"highload-counters/internal/bootstrap"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker failed", "error", err)
		os.Exit(1)
	}
}
func run() error {
	cfg, err := bootstrap.NewConfig()
	if err != nil {
		return err
	}
	deps, err := bootstrap.NewDeps(cfg)
	if err != nil {
		return err
	}
	defer deps.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return deps.Worker.Run(ctx)
}
