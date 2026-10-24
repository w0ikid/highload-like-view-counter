package session

import (
	"context"
	"github.com/jmoiron/sqlx"
	"highload-counters/internal/domain"
	"highload-counters/internal/infrastructure/postgres"
)

type Repository struct{ db *sqlx.DB }

func New(db *sqlx.DB) *Repository { return &Repository{db: db} }

var _ domain.SessionRepository = (*Repository)(nil)

func (r *Repository) Create(ctx context.Context, s domain.Session) error {
	_, err := postgres.Query(ctx, r.db).ExecContext(ctx, `INSERT INTO sessions(id,user_id,refresh_token_hash,expires_at) VALUES($1,$2,$3,$4)`, s.ID, s.UserID, s.RefreshTokenHash, s.ExpiresAt)
	return postgres.MapError(err)
}
func (r *Repository) FindByHash(ctx context.Context, hash string) (s domain.Session, err error) {
	err = postgres.Query(ctx, r.db).QueryRowxContext(ctx, `SELECT * FROM sessions WHERE refresh_token_hash=$1`, hash).StructScan(&s)
	return s, postgres.MapError(err)
}
func (r *Repository) FindByID(ctx context.Context, id string) (s domain.Session, err error) {
	err = postgres.Query(ctx, r.db).QueryRowxContext(ctx, `SELECT * FROM sessions WHERE id=$1`, id).StructScan(&s)
	return s, postgres.MapError(err)
}
func (r *Repository) Revoke(ctx context.Context, id string) error {
	_, err := postgres.Query(ctx, r.db).ExecContext(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1`, id)
	return postgres.MapError(err)
}
func (r *Repository) RevokeByUser(ctx context.Context, id int64) error {
	_, err := postgres.Query(ctx, r.db).ExecContext(ctx, `UPDATE sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id)
	return postgres.MapError(err)
}
