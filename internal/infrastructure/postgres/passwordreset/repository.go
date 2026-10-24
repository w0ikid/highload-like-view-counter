package passwordreset

import (
	"context"
	"github.com/jmoiron/sqlx"
	"highload-counters/internal/domain"
	"highload-counters/internal/infrastructure/postgres"
)

type Repository struct{ db *sqlx.DB }

func New(db *sqlx.DB) *Repository { return &Repository{db: db} }

var _ domain.PasswordResetRepository = (*Repository)(nil)

func (r *Repository) Create(ctx context.Context, p domain.PasswordReset) error {
	_, err := postgres.Query(ctx, r.db).ExecContext(ctx, `INSERT INTO password_resets(user_id,token_hash,expires_at) VALUES($1,$2,$3)`, p.UserID, p.TokenHash, p.ExpiresAt)
	return postgres.MapError(err)
}
func (r *Repository) FindByHash(ctx context.Context, hash string) (p domain.PasswordReset, err error) {
	err = postgres.Query(ctx, r.db).QueryRowxContext(ctx, `SELECT * FROM password_resets WHERE token_hash=$1`, hash).StructScan(&p)
	return p, postgres.MapError(err)
}
func (r *Repository) Consume(ctx context.Context, hash string) (bool, error) {
	result, err := postgres.Query(ctx, r.db).ExecContext(ctx, `UPDATE password_resets SET used_at=now() WHERE token_hash=$1 AND used_at IS NULL AND expires_at>clock_timestamp()`, hash)
	if err != nil {
		return false, postgres.MapError(err)
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
func (r *Repository) InvalidateByUser(ctx context.Context, id int64) error {
	_, err := postgres.Query(ctx, r.db).ExecContext(ctx, `UPDATE password_resets SET used_at=now() WHERE user_id=$1 AND used_at IS NULL`, id)
	return postgres.MapError(err)
}
