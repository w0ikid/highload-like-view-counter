package bootstrap

import (
	"context"
	"errors"
	redisstore "highload-counters/internal/infrastructure/redis"
	"highload-counters/internal/transport/rest"
	restauth "highload-counters/internal/transport/rest/auth"
	"net/http"
	"time"
)

type App struct {
	config Config
	server *http.Server
}

func NewApp(c Config, d *Deps) *App {
	mux := rest.NewRouter(func(ctx context.Context) error { return d.Ready(ctx, c) }, c.RequestTimeout)
	restauth.New(d.Auth, &redisstore.RateLimiter{Client: d.Redis, Timeout: c.RedisTimeout}, c.AuthTimeout).RegisterRoutes(mux)
	registerSwagger(mux)
	return &App{config: c, server: &http.Server{
		Addr: c.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
	}}
}

func (a *App) Run(ctx context.Context) error {
	done := make(chan error, 1)
	go func() { done <- a.server.ListenAndServe() }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.config.ShutdownTimeout)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			_ = a.server.Close()
			return err
		}
		return nil
	}
}
