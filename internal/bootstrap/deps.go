package bootstrap

import (
	"context"
	"github.com/jmoiron/sqlx"
	goredis "github.com/redis/go-redis/v9"
	"highload-counters/internal/infrastructure/mail"
	"highload-counters/internal/infrastructure/postgres"
	"highload-counters/internal/infrastructure/postgres/credential"
	"highload-counters/internal/infrastructure/postgres/passwordreset"
	"highload-counters/internal/infrastructure/postgres/session"
	"highload-counters/internal/infrastructure/postgres/user"
	redisstore "highload-counters/internal/infrastructure/redis"
	"highload-counters/internal/infrastructure/sqltx"
	"highload-counters/internal/infrastructure/token"
	"highload-counters/internal/usecase/auth"
)

type Deps struct {
	Auth  *auth.Service
	DB    *sqlx.DB
	Redis *goredis.Client
}

func NewDeps(c Config) (*Deps, error) {
	db, err := postgres.Open(c.PostgresDSN)
	if err != nil {
		return nil, err
	}
	service := auth.New(user.New(db), credential.New(db), session.New(db), passwordreset.New(db), sqltx.NewManager(db),
		token.New(c.JWTSecret, c.AccessTTL), &mail.Sender{Addr: c.SMTPAddr, From: c.MailFrom, ResetURL: c.PasswordResetURL, Timeout: c.SMTPTimeout},
		auth.Config{RefreshTTL: c.RefreshTTL, ResetTTL: c.ResetTTL, DBTimeout: c.PostgresTimeout})
	return &Deps{DB: db, Redis: redisstore.New(c.RedisAddr, c.RedisTimeout), Auth: service}, nil
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
