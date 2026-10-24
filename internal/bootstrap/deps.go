package bootstrap

import (
	"context"
	"github.com/jmoiron/sqlx"
	goredis "github.com/redis/go-redis/v9"
	"highload-counters/internal/infrastructure/postgres"
	redisstore "highload-counters/internal/infrastructure/redis"
)

type Deps struct {
	DB    *sqlx.DB
	Redis *goredis.Client
}

func NewDeps(c Config) (*Deps, error) {
	db, err := postgres.Open(c.PostgresDSN)
	if err != nil {
		return nil, err
	}
	return &Deps{DB: db, Redis: redisstore.New(c.RedisAddr, c.RedisTimeout)}, nil
}

func (d *Deps) Close() { _ = d.Redis.Close(); _ = d.DB.Close() }

func (d *Deps) Ready(ctx context.Context, c Config) error {
	pgCtx, cancel := context.WithTimeout(ctx, c.PostgresTimeout)
	err := d.DB.PingContext(pgCtx)
	cancel()
	if err != nil {
		return err
	}
	redisCtx, cancel := context.WithTimeout(ctx, c.RedisTimeout)
	defer cancel()
	return d.Redis.Ping(redisCtx).Err()
}
