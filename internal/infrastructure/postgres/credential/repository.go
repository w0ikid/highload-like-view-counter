package credential

import (
	"context"
	"github.com/jmoiron/sqlx"
	"highload-counters/internal/domain"
	"highload-counters/internal/infrastructure/postgres"
)

type Repository struct{ db *sqlx.DB }

func New(db *sqlx.DB) *Repository { return &Repository{db: db} }

var _ domain.CredentialRepository = (*Repository)(nil)

func (r *Repository) Find(ctx context.Context, id int64) (c domain.Credential, err error) {
	err = postgres.Query(ctx, r.db).QueryRowxContext(ctx, `SELECT * FROM user_credentials WHERE user_id=$1`, id).StructScan(&c)
	return c, postgres.MapError(err)
}
func (r *Repository) Set(ctx context.Context, id int64, hash string) error {
	_, err := postgres.Query(ctx, r.db).ExecContext(ctx, `INSERT INTO user_credentials(user_id,password_hash) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET password_hash=EXCLUDED.password_hash,updated_at=now()`, id, hash)
	return postgres.MapError(err)
}
