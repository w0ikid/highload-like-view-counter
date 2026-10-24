package bootstrap

import (
	"context"
	"errors"
	"google.golang.org/grpc"
	redisstore "highload-counters/internal/infrastructure/redis"
	grpctransport "highload-counters/internal/transport/grpc"
	"highload-counters/internal/transport/identity"
	"highload-counters/internal/transport/rest"
	restauth "highload-counters/internal/transport/rest/auth"
	restcounters "highload-counters/internal/transport/rest/counters"
	"net"
	"net/http"
	"time"
)

type App struct {
	config Config
	server *http.Server
	grpc   *grpc.Server
}

func NewApp(c Config, d *Deps) *App {
	mux := rest.NewRouter(func(ctx context.Context) error { return d.Ready(ctx, c) }, c.RequestTimeout)
	restauth.New(d.Auth, &redisstore.RateLimiter{Client: d.Redis, Timeout: c.RedisTimeout}, c.AuthTimeout).RegisterRoutes(mux)
	resolver := identity.Resolver{Mode: c.CounterAuthMode, Auth: d.Auth}
	(&restcounters.Handler{Service: d.Counters, Identity: resolver, Timeout: c.RequestTimeout}).RegisterRoutes(mux)
	registerSwagger(mux)
	return &App{config: c, grpc: grpctransport.New(d.Counters, resolver, c.RequestTimeout), server: &http.Server{
		Addr: c.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
	}}
}

func (a *App) Run(ctx context.Context) error {
	httpListener, err := net.Listen("tcp", a.config.HTTPAddr)
	if err != nil {
		return err
	}
	grpcListener, err := net.Listen("tcp", a.config.GRPCAddr)
	if err != nil {
		_ = httpListener.Close()
		return err
	}
	done := make(chan error, 2)
	go func() { done <- a.server.Serve(httpListener) }()
	go func() { done <- a.grpc.Serve(grpcListener) }()
	select {
	case <-ctx.Done():
	case err = <-done:
	}
	shutdown, cancel := context.WithTimeout(context.Background(), a.config.ShutdownTimeout)
	defer cancel()
	grpcDone := make(chan struct{})
	go func() { a.grpc.GracefulStop(); close(grpcDone) }()
	if stopErr := a.server.Shutdown(shutdown); stopErr != nil {
		_ = a.server.Close()
		if err == nil {
			err = stopErr
		}
	}
	select {
	case <-grpcDone:
	case <-shutdown.Done():
		a.grpc.Stop()
	}
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}
